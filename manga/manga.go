// Package manga contains common types for the manga.displaynone.us client & server.
package manga

import (
	"fmt"
	"time"
)

type (
	SeriesKind   int
	ReleaseKind  int
	SeriesStatus int
	JobType      int
)

const (
	Manga SeriesKind = iota
	Doujinshi
)

func (s SeriesKind) String() string {
	return [...]string{"Manga", "Doujinshi"}[s]
}

const (
	Chapter ReleaseKind = iota
	Volume
	Oneshot
	DramaCD
	Episode
	OVA
	Other
)

func (r ReleaseKind) String() string {
	return []string{"c", "v", "", "cd", "ep", "ova", ""}[r]
}

const (
	Ongoing SeriesStatus = iota
	Complete
	Axed
	Hiatus
)

func (s SeriesStatus) String() string {
	return []string{"In serialization", "Complete", "Complete — axed", "In hiatus"}[s]
}

const (
	Translation JobType = iota
	Cleaning
	Lettering
)

func (t JobType) String() string {
	// CSS classes
	return []string{"tl", "clean", "letter"}[t]
}

func (t JobType) LongName() string {
	return []string{"Translation", "Cleaning", "Lettering"}[t]
}

type Series struct {
	Id          int `sql:"series_id"`
	Title       string
	NativeTitle string
	RomajiTitle string
	Shortname   string
	Kind        SeriesKind `sql:"series_kind"`
	Status      SeriesStatus
	Notes       string    `sql:"series_notes"`
	DateAdded   time.Time `sql:"series_added"`
	Releases    []*Release
}

func (s *Series) LatestRelease() *Release {
	switch len(s.Releases) {
	case 1:
		if rls := s.Releases[0]; rls.Progress == nil {
			return rls
		}
		fallthrough
	case 0:
		return nil
	}
	latest := s.Releases[0]
	for _, rls := range s.Releases {
		if rls.DateAdded.After(latest.DateAdded) && rls.Progress == nil {
			latest = rls
		}
	}
	return latest
}

type SeriesList []*Series

func (s SeriesList) Len() int {
	return len(s)
}

func (s SeriesList) Less(i, j int) bool {
	a := s[i].Releases
	if len(a) == 0 {
		return false
	}
	b := s[j].Releases
	if len(b) == 0 {
		return true
	}
	return a[len(a)-1].DateAdded.After(b[len(b)-1].DateAdded)
}

func (s SeriesList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

type Release struct {
	Id        int         `sql:"release_id"`
	SeriesId  int         `sql:"release_series_id"`
	Kind      ReleaseKind `sql:"release_kind"`
	Ordinal   int
	ISBN      string
	Notes     string
	Filename  string
	Filesize  Filesize
	NSFW      bool
	HitCount  int
	DateAdded time.Time `sql:"release_added"`
	Links     []*Link
	Progress  []*Progress
}

type SeriesRelease struct {
	Id         int
	Title      string
	Shortname  string
	SeriesId   int
	SeriesKind SeriesKind
	Kind       ReleaseKind
	Ordinal    int
	ISBN       string
	Notes      string
	Filename   string
	Filesize   Filesize
	NSFW       bool
	DateAdded  time.Time
}

type Link struct {
	Id        int
	ReleaseId int
	Name      string
	URL       string
}

type Progress struct {
	Id          int `sql:"progress_id"`
	ReleaseId   int `sql:"progress_release_id"`
	Job         JobType
	Done        int
	Total       int
	LastUpdated time.Time `sql:"progress_updated"`
}

func (p *Progress) Percent() float32 {
	return 100 * float32(p.Done) / float32(p.Total)
}

type NewsPost struct {
	Id         int
	Title      string
	Body       string
	DatePosted time.Time
}

type Filesize int

const (
	KB = 1024 << (10 * iota)
	MB
	GB
)

func (f Filesize) String() string {
	switch {
	case f < 0:
		return ""
	case f < KB:
		return fmt.Sprintf("%dB", f)
	case f < MB:
		return fmt.Sprintf("%.1fk", float64(f)/KB)
	case f < GB:
		return fmt.Sprintf("%.1fM", float64(f)/MB)
	}
	return fmt.Sprintf("%.1fG", float64(f)/GB)
}
