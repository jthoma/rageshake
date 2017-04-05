/*
Copyright 2017 Vector Creations Ltd

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// logServer is an http.handler which will serve up bugreports
type logServer struct {
	root string
}

func (f *logServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upath := r.URL.Path

	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
		r.URL.Path = upath
	}

	log.Println("Serving", upath)

	// eliminate ., .., //, etc
	upath = path.Clean(upath)

	// reject some dodgy paths
	if containsDotDot(upath) || strings.Contains(upath, "\x00") || (filepath.Separator != '/' && strings.IndexRune(upath, filepath.Separator) >= 0) {
		http.Error(w, "invalid URL path", http.StatusBadRequest)
		return
	}

	// convert to abs path
	upath, err := filepath.Abs(filepath.Join(f.root, filepath.FromSlash(upath)))

	if err != nil {
		msg, code := toHTTPError(err)
		http.Error(w, msg, code)
		return
	}

	serveFile(w, r, upath)
}

func serveFile(w http.ResponseWriter, r *http.Request, path string) {
	d, err := os.Stat(path)
	if err != nil {
		msg, code := toHTTPError(err)
		http.Error(w, msg, code)
		return
	}

	// if it's a directory, or doesn't look like a gzip, serve as normal
	if d.IsDir() || !strings.HasSuffix(path, ".gz") {
		log.Println("Serving", path)
		http.ServeFile(w, r, path)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	acceptsGzip := false
	splitRune := func(s rune) bool { return s == ' ' || s == '\t' || s == '\n' || s == ',' }
	for _, hdr := range r.Header["Accept-Encoding"] {
		for _, enc := range strings.FieldsFunc(hdr, splitRune) {
			if enc == "gzip" {
				acceptsGzip = true
				break
			}
		}
	}

	if acceptsGzip {
		serveGzip(w, r, path, d.Size())
	} else {
		serveUngzipped(w, r, path)
	}
}

// serveGzip serves a gzipped file with gzip content-encoding
func serveGzip(w http.ResponseWriter, r *http.Request, path string, size int64) {
	f, err := os.Open(path)
	if err != nil {
		msg, code := toHTTPError(err)
		http.Error(w, msg, code)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))

	w.WriteHeader(http.StatusOK)
	io.Copy(w, f)
}

// serveUngzipped ungzips a gzipped file and serves it
func serveUngzipped(w http.ResponseWriter, r *http.Request, path string) {
	f, err := os.Open(path)
	if err != nil {
		msg, code := toHTTPError(err)
		http.Error(w, msg, code)
		return
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		msg, code := toHTTPError(err)
		http.Error(w, msg, code)
		return
	}
	defer gz.Close()

	w.WriteHeader(http.StatusOK)
	io.Copy(w, gz)
}

func toHTTPError(err error) (msg string, httpStatus int) {
	if os.IsNotExist(err) {
		return "404 page not found", http.StatusNotFound
	}
	if os.IsPermission(err) {
		return "403 Forbidden", http.StatusForbidden
	}
	// Default:
	return "500 Internal Server Error", http.StatusInternalServerError
}

func containsDotDot(v string) bool {
	if !strings.Contains(v, "..") {
		return false
	}
	for _, ent := range strings.FieldsFunc(v, isSlashRune) {
		if ent == ".." {
			return true
		}
	}
	return false
}
func isSlashRune(r rune) bool { return r == '/' || r == '\\' }