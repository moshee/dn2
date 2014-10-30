DROP SCHEMA manga CASCADE;

BEGIN;

SET CONSTRAINTS ALL DEFERRED;

CREATE SCHEMA manga;
SET search_path TO manga,public;

CREATE TABLE series (
    id           serial      PRIMARY KEY,
    title        text        NOT NULL,
    native_title text        NOT NULL,
    romaji_title text        NOT NULL,
    shortname    text        NOT NULL,
    kind         int         NOT NULL,
    status       int         NOT NULL,
    notes        text        NOT NULL,
    date_added   timestamptz NOT NULL
);

CREATE TABLE releases (
    id         serial      PRIMARY KEY,
    series_id  int         NOT NULL REFERENCES series,
    kind       int         NOT NULL,
    ordinal    int         NOT NULL,
    isbn       text        NOT NULL DEFAULT '',
    notes      text        NOT NULL DEFAULT '',
    filename   text        NOT NULL,
    filesize   int         NOT NULL,
    nsfw       boolean     NOT NULL DEFAULT false,
    hit_count  int         NOT NULL DEFAULT 0,
    date_added timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE buy_links (
    id         serial PRIMARY KEY,
    release_id int    NOT NULL REFERENCES releases,
    name       text   NOT NULL,
    url        text   NOT NULL
);

CREATE TABLE release_progress (
    id           serial      PRIMARY KEY,
    release_id   int         NOT NULL REFERENCES releases,
    job          int         NOT NULL,
    done         int         NOT NULL,
    total        int         NOT NULL,
    last_updated timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE news (
    id          serial      PRIMARY KEY,
    title       text        NOT NULL,
    body        text        NOT NULL,
    date_posted timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION hitcounter() RETURNS TABLE (
    title text,
    unit  text,
    ordinal int,
    hit_count int,
    date_added timestamptz
) AS $$
    SELECT
        s.title,
        u.units[r.kind+1] unit,
        r.ordinal,
		r.hit_count,
		r.date_added
    FROM
        manga.series s,
		manga.releases r,
		( SELECT array['Chapter', 'Volume', 'Oneshot', 'CD', 'Other'] units ) u
    WHERE s.id = r.series_id
    ORDER BY r.date_added DESC;
$$

END;
