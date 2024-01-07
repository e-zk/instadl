package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// config
var (
	dlPath  = "./static/posts"
	cssPath = "./style.css"
)

func constructArgs(postId, outPath string) []string {
	return []string{"--dirname-pattern=" + outPath, "--filename-pattern={profile}-{shortcode}", "--no-metadata-json", "--", "-" + postId}
}

type PostMedia struct {
	Type       string // image/video/text?
	ContentUrl string // the relative url for the server
}

type Post struct {
	Id     string
	Author string
	Media  []PostMedia
}

type Data struct {
	Results []Post
	Ids     string
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles("index.tmpl.html")
	if err != nil {
		log.Fatal(err)
	}

	// get DATA here
	idsParam := r.FormValue("ids")
	ids := strings.Split(idsParam, ",")
	if idsParam != "" {
		var posts []Post

		for _, i := range ids {
			posts = append(posts, dirToPost(filepath.Join(dlPath, i)))
		}

		data := Data{
			Results: posts,
			Ids:     idsParam,
		}

		err = t.Execute(w, data)
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	err = t.Execute(w, nil)
	if err != nil {
		log.Fatal(err)
	}

}
func handleCss(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, cssPath)
}

// download post via id and then return the path to where its downloaded
func execInstaLoader(postId string) string {
	p := path.Join(dlPath, postId)
	err := os.Mkdir(p, 0750)
	if errors.Is(err, os.ErrExist) {
		log.Println("skipping... already downloaded")
		return p
	} else if err != nil {
		log.Fatal(err)
	}

	args := constructArgs(postId, p)

	cmd := exec.Command("instaloader", args...)
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	return p
}

func dirToPost(dir string) Post {
	var media []PostMedia
	var p Post
	var author string

	rfs := os.DirFS(dir)
	fs.WalkDir(rfs, ".", func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Fatalf("d2p\t!=> walk: %v", err)
		}

		// TODO more content types
		if filepath.Ext(fpath) == ".jpg" {
			relpath, _ := strings.CutPrefix(fpath, dlPath)
			relpath = path.Join("/download", filepath.Base(dir), relpath)

			m := PostMedia{
				Type:       "image",
				ContentUrl: relpath,
			}
			media = append(media, m)
			author = strings.Split(filepath.Base(relpath), "-")[0]
		}

		return nil
	})

	p.Id = filepath.Base(dir)
	p.Author = "@" + author
	p.Media = media
	return p
}

func handleGetPost(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		links := r.PostFormValue("postlink")
		log.Printf("getpost\t=> submitted: %v", links)

		var outputIds []string

		for _, link := range strings.Split(strings.Replace(strings.Replace(links, "\n", ",", -1), "\r", "", -1), ",") {

			postId := link
			log.Printf("getpost\t=> got '%s'", link)

			// TODO do something cooler here
			if strings.HasPrefix(link, "http") {
				link = strings.Replace(link, "/reel/", "/p/", -1)
				r := regexp.MustCompile(".*/p/(.*)/.*")
				m := r.FindAllStringSubmatch(link, -1)
				log.Printf("matches\t=> %v", m)
				// TODO use regexp match of id here
				postId = m[0][1]
			}

			log.Printf("\t\t=> %s", postId)

			// download files via instaloader script
			outputPath := execInstaLoader(postId)
			log.Printf("getpost\t=> downloaded '%s'", outputPath)

			outputIds = append(outputIds, postId)
		}

		http.Redirect(w, r, "/?ids="+strings.Join(outputIds, ","), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// check string is in []string
func isIn(str string, check []string) bool {
	for _, x := range check {
		if str == x {
			return true
		}
	}
	return false
}

func handleZipPost(w http.ResponseWriter, r *http.Request) {
	// get Ids from form
	idsParam := r.FormValue("ids")
	if idsParam == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	ids := strings.Split(idsParam, ",")

	// get list of paths that need to be included in zip based on the Ids we have
	var files []string
	rfs := os.DirFS(dlPath)
	fs.WalkDir(rfs, ".", func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Fatalf("walk\t!=> %v", err)
		}

		if d.IsDir() && strings.Count(fpath, string(os.PathSeparator)) > 1 {
			return nil
		}

		if isIn(filepath.Base(fpath), ids) {
			log.Printf("zip\t=> %s", fpath)
			files = append(files, path.Join(dlPath, fpath))
		}

		return nil
	})

	// create archive + zip writer
	archive, err := os.CreateTemp(dlPath, "*.zip")
	if err != nil {
		log.Fatal(err)
	}
	zw := zip.NewWriter(archive)

	// add each file to the zip
	for _, f := range files {
		pfs := os.DirFS(f)
		fs.WalkDir(pfs, ".", func(fpath string, d fs.DirEntry, err error) error {
			if fpath == "." {
				return nil
			}

			log.Printf("zip\t=> adding '%s' to archive", fpath)
			w, err := zw.Create(filepath.Join(filepath.Base(f), fpath))
			if err != nil {
				log.Fatal(err)
			}

			// open file
			fd, err := os.Open(filepath.Join(f, fpath))
			if err != nil {
				log.Fatal(err)
			}

			// write to archive
			if _, err := io.Copy(w, fd); err != nil {
				log.Fatal(err)
			}
			fd.Close()

			return nil
		})
	}

	// close writer + archive file before redirecting client to download the file
	zw.Close()
	archive.Close()

	http.Redirect(w, r, "/download/"+filepath.Base(archive.Name()), http.StatusSeeOther)
}

func main() {
	mux := http.NewServeMux()

	mux.Handle("/download/", http.StripPrefix("/download", http.FileServer(http.Dir(dlPath))))
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/style.css", handleCss)
	mux.HandleFunc("/getpost", handleGetPost)
	mux.HandleFunc("/getzip", handleZipPost)

	log.Println("starting...")
	http.ListenAndServe(":8585", mux)
}
