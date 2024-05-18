package main

import (
	"errors"
	"flag"
	"github.com/russross/blackfriday"
	"github.com/sevlyar/go-daemon"
	"html/template"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Config is a type representing the configuration for a server.
//
// The fields in this type are:
// - wwwRoot: The root directory for serving static files.
// - daemonize: A boolean indicating whether to run the server as a daemon.
// - bind: The address to bind the server to.
//
// Example usage:
//
//	cfg := Config{
//	  wwwRoot:   "/var/www/html",
//	  daemonize: true,
//	  bind:      "127.0.0.1:8080",
//	}
type Config struct {
	wwwRoot   string // The root directory for serving static files.
	daemonize bool   //A boolean indicating whether to run the server as a daemon.
	bind      string //The address to bind the server to.
}

var cfg Config

var outputTemplate = template.Must(template.New("base").Parse(`
<html>
	<head>
		<title>{{ .Path }}</title>
		<link rel="stylesheet" href="https://cdn.datatables.net/2.0.7/css/dataTables.dataTables.css" />
		<script src="https://code.jquery.com/jquery-3.7.1.min.js" integrity="sha256-/JqT3SQfawRcv/BIHPThkBvs0OEvtFFmqPF/lYI/Cxo="crossorigin="anonymous"></script>
		<script src="https://cdn.datatables.net/2.0.7/js/dataTables.js"></script>
		<script>
			$(document).ready(function() {
				$('table').addClass('display');
				$('table').DataTable({
					searching: true,
					ordering: true,
					iDisplayLength: 200,
					stateSave:true
				});
			});
		</script>
	</head>
	<body>
		{{ .Body }}
	</body>
</html>
`))

type renderer struct {
	d http.Dir
	h http.Handler
}

func main() {
	flag.StringVar(&cfg.wwwRoot, "r", ".", "root directory")
	flag.BoolVar(&cfg.daemonize, "b", false, "run in background/daemonize")
	flag.StringVar(&cfg.bind, "bind", "127.0.0.1:19000", "port to run the server on")
	flag.Parse()

	pth, _ := filepath.Abs(cfg.wwwRoot)
	log.Println("www root:", pth)

	httpDir := http.Dir(pth)
	handler := renderer{httpDir, http.FileServer(httpDir)}

	log.Println("Serving on http://" + cfg.bind)

	if cfg.daemonize {
		cnTxt := &daemon.Context{
			WorkDir: "./",
			Umask:   027,
		}

		d, err := cnTxt.Reborn()
		if err != nil {
			log.Fatal("Unable to run: ", err)
		}
		if d != nil {
			return
		}
		defer func(cnTxt *daemon.Context) {
			err := cnTxt.Release()
			if err != nil {
				log.Println(err)
			}
		}(cnTxt)
	}

	log.Print("- - - - - - - - - - - - - - -")
	log.Print("daemon started")

	log.Fatal(http.ListenAndServe(cfg.bind, handler))
}

func (r renderer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

	req.URL.Path = path.Clean(req.URL.Path)

	if strings.HasSuffix(req.URL.Path, "/") {
		if _, err := os.Stat(cfg.wwwRoot + req.URL.Path + "top.md"); err == nil {
			req.URL.Path = req.URL.Path + "top.md"
		}
		if _, err := os.Stat(cfg.wwwRoot + req.URL.Path + "index.md"); err == nil {
			req.URL.Path = req.URL.Path + "index.md"
		}
	}
	if !strings.HasSuffix(req.URL.Path, ".md") && !strings.HasSuffix(req.URL.Path, "/guide") {
		r.h.ServeHTTP(rw, req)
		return
	}

	var pathErr *os.PathError
	input, err := os.ReadFile(cfg.wwwRoot + req.URL.Path)
	if errors.As(err, &pathErr) {
		http.Error(rw, http.StatusText(http.StatusNotFound)+": "+req.URL.Path, http.StatusNotFound)
		log.Printf("file not found: %s", err)
		return
	}
	log.Println(req.RemoteAddr + " " + req.Method + " " + "\"" + req.URL.Path + "\"")

	if err != nil {
		http.Error(rw, "Internal Server Error: "+err.Error(), 500)
		log.Printf("Couldn't read path %s: %v (%T)", req.URL.Path, err, err)
		return
	}

	output := blackfriday.MarkdownCommon(input)

	rw.Header().Set("Content-Type", "text/html; charset=utf-8")

	err = outputTemplate.Execute(rw, struct {
		Path string
		Body template.HTML
	}{
		Path: req.URL.Path,
		Body: template.HTML(output),
	})
	if err != nil {
		log.Println(err)
	}

}
