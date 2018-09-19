package ftpSync

import (
	"errors"
	"github.com/jonnywang/go-kits/redis"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

type ftpSyncRedisHandler struct {
	redis.RedisHandler
}

func (obj *ftpSyncRedisHandler) Init() error {
	obj.Initiation(func() {
		GSyncFtp.Init()
	})

	return nil
}

func (obj *ftpSyncRedisHandler) Shutdown() {
	Logger.Print("redis server will shutdown")
}

func (obj *ftpSyncRedisHandler) Version() (string, error) {
	return VERSION, nil
}

func (obj *ftpSyncRedisHandler) Ping(message string) (string, error) {
	if len(message) > 0 {
		return message, nil
	}

	return "PONG", nil
}

func (obj *ftpSyncRedisHandler) FtpSync(localFile, remoteFile string) error {
	if len(localFile) == 0 || len(remoteFile) == 0 || strings.HasSuffix(remoteFile, "/") {
		return errors.New("error params")
	}

	if GSyncFtp.Sync(localFile, remoteFile, 1) {
		return nil
	}

	return errors.New("sync fail")
}

func (obj *ftpSyncRedisHandler) Files(remoteFolder string, recursionShow int) ([]string, error) {
	if len(remoteFolder) == 0 || strings.HasPrefix(remoteFolder, "/") == false {
		return nil, errors.New("error params")
	}

	return GSyncFtp.ListFiles(remoteFolder, recursionShow)
}

func (obj *ftpSyncRedisHandler) Delete(remoteFile string) error {
	if len(remoteFile) == 0 || strings.HasPrefix(remoteFile, "/") == false || strings.HasSuffix(remoteFile, "/") == true {
		return errors.New("error params")
	}

	go GSyncFtp.DeleteFile(remoteFile)

	return nil
}

func Run() {
	wordsFilterHandler := &ftpSyncRedisHandler{}

	err := wordsFilterHandler.Init()
	if err != nil {
		Logger.Print(err)
		return
	}

	wordsFilterServer, err := redis.NewServer(GConfig.ListenServer, wordsFilterHandler)
	if err != nil {
		Logger.Print(err)
		return
	}

	serverStop := make(chan bool)
	stopSignal := make(chan os.Signal)
	signal.Notify(stopSignal, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-stopSignal
		wordsFilterServer.Stop(10)
		GSyncFtp.Stop()
		serverStop <- true
	}()

	err = wordsFilterServer.Start()
	if err != nil {
		Logger.Print(err)
		stopSignal <- syscall.SIGTERM
	}

	<-serverStop

	close(serverStop)
	close(stopSignal)

	Logger.Print("all server shutdown")
}
