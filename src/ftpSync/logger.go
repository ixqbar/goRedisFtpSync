package ftpSync

import (
	"github.com/jonnywang/go-kits/redis"
	"io"
	"log"
	"os"
)

func init() {
	redis.Logger.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

var Logger = redis.Logger

type LogWriter struct {
	name string
	l    *log.Logger
	w    io.Writer
}

func NewLogWriter(name string) *LogWriter {
	return &LogWriter{
		name: name,
		l:    log.New(os.Stdout, "", log.Ldate|log.Ltime),
	}
}

func (w *LogWriter) Write(d []byte) (int, error) {
	w.l.Printf("%s|%s", w.name, string(d))
	return 0, nil
}
