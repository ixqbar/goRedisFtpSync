package ftpSync

import (
	"github.com/jonnywang/go-kits/redis"
	"log"
)

func init() {
	redis.Logger.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}
