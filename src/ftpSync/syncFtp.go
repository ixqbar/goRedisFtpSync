package ftpSync

import (
	"errors"
	"github.com/jlaffaye/ftp"
	"os"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"
)

type SyncFileInfo struct {
	LocalFile   string
	RemoteFile  string
	NumberTimes int
}

func NewSyncFileInfo(localFile, remoteFile string, numberTimes int) *SyncFileInfo {
	return &SyncFileInfo{
		LocalFile:   localFile,
		RemoteFile:  remoteFile,
		NumberTimes: numberTimes,
	}
}

type SyncFtp struct {
	sync.Mutex
	syncFileChannel chan *SyncFileInfo
	syncStopChannel chan bool
	allRemoteFolder map[string]bool
	syncFtpServer   *ftp.ServerConn
}

func (obj *SyncFtp) Init() {
	obj.syncFtpServer = nil
	obj.allRemoteFolder = make(map[string]bool, 0)
	obj.syncFileChannel = make(chan *SyncFileInfo, 10)
	obj.syncStopChannel = make(chan bool, 0)

	obj.Refresh()

	go func() {
		checkInterval := time.NewTicker(time.Second * time.Duration(10))
		defer func() {
			Logger.Print("syncFtp will stop")
			checkInterval.Stop()
			close(obj.syncStopChannel)
			close(obj.syncFileChannel)

			if obj.syncFtpServer != nil {
				obj.syncFtpServer.Logout()
				obj.syncFtpServer.Quit()
			}
		}()

	E:
		for {
			select {
			case <-checkInterval.C:
				obj.Refresh()
			case syncFile := <-obj.syncFileChannel:
				obj.Put(syncFile.LocalFile, syncFile.RemoteFile, syncFile.NumberTimes)
			case <-obj.syncStopChannel:
				Logger.Print("syncFtp catch stop signal")
				break E
			}
		}

	F:
		for {
			select {
			case syncFile := <-obj.syncFileChannel:
				obj.Put(syncFile.LocalFile, syncFile.RemoteFile, syncFile.NumberTimes)
			default:
				break F
			}
		}
	}()
}

func (obj *SyncFtp) ftpServerStateIsActive() bool {
	if obj.syncFtpServer != nil {
		err := obj.syncFtpServer.NoOp()
		if err == nil {
			return true
		} else {
			obj.syncFtpServer = nil
			Logger.Print(err)
		}
	}

	fs, err := ftp.DialTimeout(GConfig.FtpServerAddress, time.Second*10)
	if err != nil {
		Logger.Printf("ftp server %s connect failed %v", GConfig.FtpServerAddress, err)
		return false
	}

	err = fs.Login(GConfig.FtpServerUser, GConfig.FtpServerPassword)
	if err != nil {
		Logger.Printf("ftp server %s login failed %v", GConfig.FtpServerAddress, err)
		Logger.Println(err)
		return false
	}

	obj.syncFtpServer = fs

	Logger.Printf("ftp server %s connect success", GConfig.FtpServerAddress)

	return true
}

func (obj *SyncFtp) Refresh() {
	obj.Lock()
	defer obj.Unlock()

	obj.ftpServerStateIsActive()
	Logger.Printf("current load ftp server folders %v", reflect.ValueOf(obj.allRemoteFolder).MapKeys())
}

func (obj *SyncFtp) Put(localFile, remoteFile string, numberTimes int) bool {
	obj.Lock()
	defer obj.Unlock()

	oldRemoteFile := remoteFile

	if strings.HasPrefix(remoteFile, "/") {
		remoteFile = remoteFile[1:]
	}

	if len(remoteFile) > 0 && strings.HasSuffix(remoteFile, "/") {
		remoteFile = remoteFile[:len(remoteFile)-1]
	}

	if len(remoteFile) == 0 {
		Logger.Printf("sync remote file %s failed", oldRemoteFile)
		return false
	}

	position := strings.LastIndex(remoteFile, "/")
	if position >= 0 {
		remoteFileFolder := remoteFile[:position]
		if _, ok := obj.allRemoteFolder[remoteFileFolder]; ok == false || numberTimes > 1 {
			remoteFileFolders := strings.Split(remoteFileFolder, "/")
			tempRemoteFileFolder := "/"
			for _, f := range remoteFileFolders {
				tempRemoteFileFolder = path.Join(tempRemoteFileFolder, f)
				if _, ok := obj.allRemoteFolder[tempRemoteFileFolder]; ok == false || numberTimes > 1 {
					err := obj.syncFtpServer.MakeDir(tempRemoteFileFolder)
					if err == nil {
						obj.allRemoteFolder[tempRemoteFileFolder] = true
						Logger.Printf("mkdir %s success", tempRemoteFileFolder)
					} else {
						Logger.Printf("mkdir %s failed %v", tempRemoteFileFolder, err)
					}
				}
			}
		}
	}

	if obj.ftpServerStateIsActive() == false {
		Logger.Printf("sync %s failed can't connected ftp server", localFile)
		obj.Sync(localFile, remoteFile, numberTimes+1)
		return false
	}

	reader, err := os.Open(localFile)
	if err != nil {
		Logger.Printf("open %s failed %v", localFile, err)
		return false
	}
	defer reader.Close()

	err = obj.syncFtpServer.Stor(remoteFile, reader)
	if err != nil {
		Logger.Printf("sync %s failed %v", localFile, err)
		obj.Sync(localFile, remoteFile, numberTimes+1)
		return false
	}

	Logger.Printf("sync %s to %s success", localFile, remoteFile)

	return true
}

func (obj *SyncFtp) Sync(localFile, remoteFile string, numberTimes int) bool {
	if numberTimes > 3 {
		Logger.Printf("sync %s to %s overflow max num", localFile, remoteFile)
		return false
	}

	if obj.syncFtpServer != nil {
		obj.syncFileChannel <- NewSyncFileInfo(localFile, remoteFile, numberTimes)
		return true
	}

	return false
}

func (obj *SyncFtp) ListFiles(remoteFolder string, recursion int) ([]string, error) {
	obj.Lock()
	defer obj.Unlock()

	if obj.ftpServerStateIsActive() == false {
		return nil, errors.New("can't connected ftp server")
	}

	if len(remoteFolder) > 1 && strings.HasSuffix(remoteFolder, "/") {
		remoteFolder = remoteFolder[:len(remoteFolder)-1]
	}

	return obj.listFtpServerFolder(remoteFolder, recursion), nil
}

func (obj *SyncFtp) listFtpServerFolder(p string, recursion int) []string {
	fileEntryList, err := obj.syncFtpServer.List(p)
	if err != nil {
		obj.syncFtpServer = nil
		Logger.Print(err)
		return nil
	}

	folderFiles := make([]string, 0)

	for _, e := range fileEntryList {
		if e.Type == ftp.EntryTypeLink || InStringArray(e.Name, []string{".", ".."}) {
			continue
		}

		tp := path.Join(p, e.Name)

		if e.Type == ftp.EntryTypeFolder {
			obj.allRemoteFolder[tp] = true
		}

		folderFiles = append(folderFiles, tp)

		if recursion == 1 && e.Type == ftp.EntryTypeFolder {
			tf := obj.listFtpServerFolder(tp, recursion)
			if tf != nil && len(tf) > 0 {
				folderFiles = append(folderFiles, tf...)
			}
		}
	}

	return folderFiles
}

func (obj *SyncFtp) DeleteFile(remoteFile string) bool {
	obj.Lock()
	defer obj.Unlock()

	if obj.ftpServerStateIsActive() == false {
		return false
	}

	err := obj.syncFtpServer.Delete(remoteFile)
	if err != nil {
		Logger.Printf("delete ftp server file %s failed %v", remoteFile, err)
		return false
	}

	Logger.Printf("delete ftp server file %s success", remoteFile)

	return true
}

func (obj *SyncFtp) ExistsFile(remoteFile string) bool {
	obj.Lock()
	defer obj.Unlock()

	if obj.ftpServerStateIsActive() == false {
		return false
	}

	_, err := obj.syncFtpServer.FileSize(remoteFile)
	if err != nil {
		Logger.Printf("check ftp server file %s exists failed %v", remoteFile, err)
		return false
	}

	return true
}

func (obj *SyncFtp) Stop() {
	obj.syncStopChannel <- true
}

var GSyncFtp = &SyncFtp{}
