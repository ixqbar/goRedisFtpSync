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

func (obj *ftpSyncRedisHandler) FtpAsync(localFile, remoteFile string) error {
	if len(localFile) == 0 || len(remoteFile) == 0 || strings.HasSuffix(remoteFile, "/") {
		return errors.New("error params")
	}

	if GSyncFtp.Sync(localFile, remoteFile, 1) {
		return nil
	}

	return errors.New("sync fail")
}

func (obj *ftpSyncRedisHandler) FtpSync(localFile, remoteFile string) error {
	if len(localFile) == 0 || len(remoteFile) == 0 || strings.HasSuffix(remoteFile, "/") {
		return errors.New("error params")
	}

	if GSyncFtp.Put(localFile, remoteFile, 1) {
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

func (obj *ftpSyncRedisHandler) Delete(remoteFile string, doAsync int) (int, error) {
	if len(remoteFile) == 0 || strings.HasPrefix(remoteFile, "/") == false || strings.HasSuffix(remoteFile, "/") == true {
		return 0, errors.New("error params")
	}

	if doAsync == 1 {
		go GSyncFtp.DeleteFile(remoteFile)
	} else {
		if GSyncFtp.DeleteFile(remoteFile) == false {
			return 0, nil
		}
	}

	return 1, nil
}

func (obj *ftpSyncRedisHandler) Exists(remoteFile string) (int, error) {
	if len(remoteFile) == 0 || strings.HasPrefix(remoteFile, "/") == false || strings.HasSuffix(remoteFile, "/") == true {
		return 0, errors.New("error params")
	}

	if GSyncFtp.ExistsFile(remoteFile) {
		return 1, nil
	}

	return 0, nil
}

func Run() {
	ftpSyncHandler := &ftpSyncRedisHandler{}

	err := ftpSyncHandler.Init()
	if err != nil {
		Logger.Print(err)
		return
	}

	ftpSyncServer, err := redis.NewServer(GConfig.ListenServer, ftpSyncHandler)
	if err != nil {
		Logger.Print(err)
		return
	}

	serverStop := make(chan bool)
	stopSignal := make(chan os.Signal)
	signal.Notify(stopSignal, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-stopSignal
		Logger.Print("catch exit signal")
		ftpSyncServer.Stop(10)
		GSyncFtp.Stop()
		serverStop <- true
	}()

	err = ftpSyncServer.Start()
	if err != nil {
		Logger.Print(err)
		stopSignal <- syscall.SIGTERM
	}

	<-serverStop

	close(serverStop)
	close(stopSignal)

	Logger.Print("all server shutdown")
}
