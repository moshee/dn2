package main

import (
	"database/sql"
	"dn2/manga"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/moshee/gas"
)

const staticDir = http.Dir("/root/projects/go/src/displaynone/static")

var fileServer = http.FileServer(staticDir)

var Env struct {
	FileRoot     string `default:"/root/manga"`
	DLServerURL  string `default:".dl.displaynone.us"`
	DLServerPort int    `default:"40001"`
}

func main() {
	if err := gas.EnvConf(&Env, "DISPLAYNONE_"); err != nil {
		gas.LogFatal("%v", err)
	}
	gas.InitDB()

	/* TODO: cache the series with a lock
	gas.Init(func() {
		series = gas.QueryJoin(...)
	})
	*/

	router := gas.New("manga.displaynone.us")
	router.Get("/rss", getRSS)
	router.Get("/get/{id}", getGet)
	//router.Get("/series/{id}", getSeries)
	//router.Get("/series", redirect("/", 302))

	router.Get("/news/{id}", getNews)
	router.Post("/news/update", updateNews)
	router.Post("/news/create", createNews)

	router.Post("/release/create", createRelease)

	router.Post("/proxy/send", postProxyClient)
	router.Get("/proxy/request/{filename}", getProxyServer)

	router.Get("/", getIndex)

	gas.New("static.displaynone.us").Get("/{path}", getStatic)

	go proxyListener()

	gas.Ignition(nil)
}

func getStatic(g *gas.Gas) (int, gas.Outputter) {
	g.Header().Set("Expires", time.Now().AddDate(0, 1, 0).Format(http.TimeFormat))
	fileServer.ServeHTTP(g, g.Request)
	return -1, nil
}

func redirect(path string, code int) gas.Handler {
	return func(g *gas.Gas) (int, gas.Outputter) {
		return code, gas.Redirect(path)
	}
}

func queryLatest(n int) ([]manga.SeriesRelease, error) {
	s := make([]manga.SeriesRelease, 0, n)
	err := gas.QueryJoin(&s, `
		SELECT
			r.id,
			s.title,
			s.shortname,
			r.series_id,
			s.kind series_kind,
			r.kind,
			r.ordinal,
			r.isbn,
			r.notes,
			r.filename,
			r.filesize,
			r.nsfw,
			MAX(r.date_added) date_added
		FROM
			manga.series s,
			manga.releases r
		WHERE r.series_id = s.id
		GROUP BY
			r.id,
			s.title,
			s.shortname,
			r.series_id,
			s.kind,
			r.kind,
			r.ordinal,
			r.isbn,
			r.notes,
			r.filename,
			r.filesize,
			r.nsfw
		ORDER BY r.date_added DESC
		LIMIT $1`, n)

	return s, err
}

func releaseLinks(s manga.SeriesList) error {
	links := make([]manga.Link, 0, len(s)*2)
	err := gas.Query(&links, `
	SELECT
		id,
		release_id,
		name,
		url
	FROM
		manga.buy_links
	ORDER BY
		release_id,
		name`)

	if err != nil {
		return err
	}

	for _, series := range s {
		for _, release := range series.Releases {
			for i := range links {
				if links[i].ReleaseId == release.Id {
					if release.Links == nil {
						release.Links = []*manga.Link{&links[i]}
					} else {
						release.Links = append(release.Links, &links[i])
					}
				}
			}
		}
	}

	return nil
}

func getIndex(g *gas.Gas) (int, gas.Outputter) {
	series := make(manga.SeriesList, 0)
	if err := gas.QueryJoin(&series, `
	SELECT
		s.id series_id,
		s.title,
		s.native_title,
		s.romaji_title,
		s.shortname,
		s.kind       series_kind,
		s.status,
		s.notes      series_notes,
		s.date_added series_added,
		r.id         release_id,
		r.series_id  release_series_id,
		r.kind       release_kind,
		r.ordinal,
		r.isbn,
		r.notes,
		r.filename,
		r.filesize,
		r.nsfw,
		r.hit_count,
		r.date_added release_added,
		p.id         progress_id,
		p.release_id progress_release_id,
		p.job,
		p.done,
		p.total,
		p.last_updated progress_updated
	FROM
		manga.series s,
		manga.releases r
		LEFT JOIN manga.release_progress p
			ON p.release_id = r.id
	WHERE s.id = r.series_id
	ORDER BY
		s.id,
		r.date_added,
		p.job`); err != nil {
		return 500, g.Error(err)
	}

	if err := releaseLinks(series); err != nil {
		return 500, g.Error(err)
	}

	sort.Sort(series)

	latest, err := queryLatest(5)
	if err != nil {
		return 500, g.Error(err)
	}

	news := new(manga.NewsPost)
	gas.Query(news, `
	SELECT
		id,
		title,
		body,
		date_posted
	FROM
		manga.news
	ORDER BY date_posted DESC
	LIMIT 1`)

	return 200, gas.HTML("index", &struct {
		Series manga.SeriesList
		News   *manga.NewsPost
		Latest []manga.SeriesRelease
	}{
		series,
		news,
		latest,
	})
}

func getRSS(g *gas.Gas) (int, gas.Outputter) {
	latest, err := queryLatest(10)
	if err != nil {
		return 500, g.Error(err)
	}

	g.Header().Set("Content-Type", "application/rss+xml")
	return 200, gas.HTML("rss", &struct {
		Now      time.Time
		Releases []manga.SeriesRelease
	}{
		time.Now(),
		latest,
	})
}

func getGet(g *gas.Gas) (int, gas.Outputter) {
	srv := "us"
	port := fmt.Sprintf(":%d", Env.DLServerPort)

	cont, err := geocontinent(g.Request.RemoteAddr)
	if err == nil {
		switch cont {
		case "NA", "SA", "OC":
			srv = "us"
			port = ":80"
		default:
			srv = "eu"
		}
	}

	id, err := g.IntArg("id")
	if err != nil {
		return 400, g.Error(err)
	}
	var filename string
	err = gas.DB.QueryRow(`
	   UPDATE manga.releases
	      SET hit_count = hit_count+1
	    WHERE id = $1
	RETURNING filename`, id).Scan(&filename)
	if err != nil {
		return 500, g.Error(err)
	}

	url := fmt.Sprintf("http://%s%s%s/%s", srv, Env.DLServerURL, port, filename)
	return 302, gas.Redirect(url)
}

type Error struct {
	Msg string
	Err error
}

// Remove files that were downloaded
func rollback(files ...string) {
	for _, file := range files {
		os.Remove(file)
	}
}

func createRelease(g *gas.Gas) (int, gas.Outputter) {
	if err := g.ParseMultipartForm(0); err != nil {
		return 500, gas.JSON(&Error{"parsing form", err})
	}

	localFiles := make([]string, 0, 3)

	for _, field := range []string{"archive", "cover", "thumb"} {
		if name, err := download(g, field); err != nil {
			g.MultipartForm.RemoveAll()
			os.Remove(name)
			rollback(localFiles...)
			return 500, gas.JSON(&Error{"downloading files", err})
		} else {
			localFiles = append(localFiles, name)
		}
	}

	blob := strings.NewReader(g.PostFormValue("data"))
	release := new(manga.Release)
	if err := json.NewDecoder(blob).Decode(release); err != nil {
		rollback(localFiles...)
		return 500, gas.JSON(&Error{"decoding json", err})
	}

	sid := -1
	if err := gas.DB.QueryRow(`
	SELECT id
	FROM manga.series
	WHERE id = $1`, release.SeriesId).Scan(&sid); err != nil {
		if err == sql.ErrNoRows {
			return 400, gas.JSON(&Error{
				"updating database",
				fmt.Errorf("Series id %d is not present in the database", release.SeriesId),
			})
		} else {
			return 500, gas.JSON(&Error{"updating database", err})
		}
	}

	_, err := gas.DB.Exec(`
	INSERT INTO manga.releases (
		series_id,
		kind,
		ordinal,
		isbn,
		notes,
		filename,
		filesize,
		nsfw
	)
	VALUES ( $1, $2, $3, $4, $5, $6, $7, $8 )`,
		release.SeriesId, release.Kind, release.Ordinal, release.ISBN,
		release.Notes, release.Filename, release.Filesize, release.NSFW)

	if err != nil {
		return 500, gas.JSON(&Error{"updating database", err})
	}

	if release.Links != nil && len(release.Links) > 0 {
		tx, err := gas.DB.Begin()
		if err != nil {
			return 500, gas.JSON(&Error{"updating database", err})
		}
		for _, link := range release.Links {
			_, err = gas.DB.Exec(`
			INSERT INTO manga.buy_links (
				release_id,
				name,
				url
			)
			VALUES ( $1, $2, $3 )`, release.Id, link.Name, link.URL)
			if err != nil {
				return 500, gas.JSON(&Error{"updating database", err})
			}
		}
		if err = tx.Commit(); err != nil {
			return 500, gas.JSON(&Error{"updating database", err})
		}
	}

	return 204, nil
}

// Assumes parsing form with 0 memory (all to disk)
func download(g *gas.Gas, field string) (string, error) {
	formFile, fh, err := g.FormFile(field)
	if err != nil {
		return "", err
	}

	file := formFile.(*os.File)
	path := filepath.Join(Env.FileRoot, fh.Filename)
	return path, os.Rename(file.Name(), path)
}

func getNews(g *gas.Gas) (int, gas.Outputter) {
	id, err := g.IntArg("id")
	if err != nil {
		return 400, gas.JSON(&Error{"bad post id", err})
	}
	post := new(manga.NewsPost)
	err = gas.Query(post, `SELECT * FROM manga.news WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return 404, gas.JSON(&Error{"no such post", err})
		}
		return 500, gas.JSON(&Error{"reading database", err})
	}

	return 200, gas.JSON(post)
}

func createNews(g *gas.Gas) (int, gas.Outputter) {
	post := new(manga.NewsPost)
	if err := json.NewDecoder(g.Body).Decode(post); err != nil {
		return 400, gas.JSON(&Error{"bad json", err})
	}

	id := -1

	err := gas.Query(&id, `
	INSERT INTO
		books.news (
			title,
			body,
			date_posted
		)
	VALUES ( $1, $2, now() )
	RETURNING id`, post.Title, post.Body)

	if err != nil {
		return 500, gas.JSON(&Error{"updating database", err})
	}

	post.Id = id
	return 200, gas.JSON(post)
}

func updateNews(g *gas.Gas) (int, gas.Outputter) {
	post := new(manga.NewsPost)
	if err := json.NewDecoder(g.Body).Decode(post); err != nil {
		return 400, gas.JSON(&Error{"bad json", err})
	}

	_, err := gas.DB.Exec(`
	UPDATE books.news
	SET
		title = $1,
		body = $2
	WHERE id = $3`, post.Title, post.Body, post.Id)
	if err != nil {
		return 500, gas.JSON(&Error{"updating database", err})
	}

	return 200, gas.JSON(post)
}
