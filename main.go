package main

import (
	"archive/zip"
	"flag"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

type PostMedia struct {
	Type       string // image | video
	ContentUrl string // the relative url for the server
	Thumb      string
}

type Post struct {
	Id          string
	Author      string
	Media       []PostMedia
	Description string
}

// data passed to template
type Data struct {
	Results []Post
	Ids     string
}

func replaceExt(path, newExt string) string {
	newPath, _ := strings.CutSuffix(path, filepath.Ext(path))
	newPath = newPath + newExt
	return newPath
}

// walk a directory, turn it into a post
func dirToPost(dir string) Post {
	var media []PostMedia
	var p Post
	var author, desc string

	rfs := os.DirFS(dir)
	fs.WalkDir(rfs, ".", func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Fatalf("d2p\t!=> walk: %v", err)
		}

		switch filepath.Ext(fpath) {
		case ".jpg":
			relpath, _ := strings.CutPrefix(fpath, dlPath)
			relpath = filepath.Join("/download", filepath.Base(dir), relpath)

			m := PostMedia{
				Type:       "image",
				ContentUrl: relpath,
			}
			media = append(media, m)

			// if author not set, set it
			if author == "" {
				author = strings.Split(filepath.Base(relpath), "-")[0]
			}
		case ".mp4":
			relpath, _ := strings.CutPrefix(fpath, dlPath)
			relpath = filepath.Join("/download", filepath.Base(dir), relpath)

			var thumblink string
			thumb := filepath.Join(dlPath, filepath.Base(dir), replaceExt(fpath, ".jpg"))
			if _, err := os.Stat(thumb); err == nil {
				thumblink = replaceExt(relpath, ".jpg")
			}

			m := PostMedia{
				Type:       "video",
				ContentUrl: relpath,
				Thumb:      thumblink,
			}
			media = append(media, m)

			// if author not set, set it
			if author == "" {
				author = strings.Split(filepath.Base(relpath), "-")[0]
			}
		case ".txt":
			// set post description
			descFile := filepath.Join(dlPath, filepath.Base(dir), fpath)
			descB, err := os.ReadFile(descFile)
			if err != nil {
				log.Fatalf("d2p\t!=> %v", err)
			}
			desc = string(descB)
		default:
			// nothing
		}

		return nil
	})

	p.Id = filepath.Base(dir)
	p.Author = "@" + author
	p.Media = media
	p.Description = desc
	return p
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles("index.tmpl.html")
	if err != nil {
		log.Fatal(err)
	}

	// get DATA here
	idsParam := r.FormValue("ids")
	if idsParam != "" {
		ids := strings.Split(idsParam, ",")

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

func handleGetPost(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		// remove carriage returns + make csv
		links := r.PostFormValue("postlink")
		links = strings.Replace(links, "\r", "", -1)
		links = strings.Replace(links, "\n", ",", -1)
		linkSlice := strings.Split(links, ",")

		log.Printf("getpost\t=> submitted: %v", linkSlice)

		var outputIds []string
		for _, link := range linkSlice {
			if link == "" {
				continue
			}

			// the link is the post's Id
			// (not really, but we make sure it is below)
			postId := link

			// if the link is in fact, a link, we need to normalise + extract the shortcode
			if strings.HasPrefix(link, "http") {
				link = strings.Replace(link, "/reel/", "/p/", -1)
				r := regexp.MustCompile(".*/p/(.*)/.*")
				m := r.FindAllStringSubmatch(link, -1)
				postId = m[0][1]
			}

			// download files via instaloader script
			log.Printf("getpost\t=> downloading %q", postId)
			_, err := execInstaLoader(postId)
			if err == ErrPostExists {
				log.Printf("getpost\t=> %q: already downloaded", postId)
			} else if err != nil {
				log.Fatal(err)
			} else {
				log.Printf("getpost\t=> downloaded %q", postId)
			}

			outputIds = append(outputIds, postId)
		}

		// go back to index w/ list of Ids in param
		http.Redirect(w, r, "/?ids="+strings.Join(outputIds, ","), http.StatusSeeOther)
		return
	}

	// go back to index
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleZipPost(w http.ResponseWriter, r *http.Request) {
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

		if slices.Contains(ids, filepath.Base(fpath)) {
			log.Printf("zip\t=> %s", fpath)
			files = append(files, filepath.Join(dlPath, fpath))
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

			log.Printf("zip\t=> adding %q to archive", fpath)
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

func handleCss(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, cssPath)
}

var (
	listenAddr string
	staticDir  string
	cssPath    string
	dlPath     string
)

func init() {
	flag.StringVar(&listenAddr, "l", ":8585", "listen address")
	flag.StringVar(&staticDir, "d", "./static", "local path to where /static is. this is where posts are downloaded to.")
	flag.StringVar(&cssPath, "s", "", "path to style.css. defaults to style.css in the directory specified by -d")
	flag.Parse()

	dlPath = filepath.Join(staticDir, "posts")
	if cssPath == "" {
		cssPath = filepath.Join(staticDir, "style.css")
	}
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
