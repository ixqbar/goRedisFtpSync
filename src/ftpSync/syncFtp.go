package ftpSync

import (
	"errors"
	"github.com/jonnywang/ftp"
	"io"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"sync"
	"syscall"
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
	activeDeadline  time.Time
	dependProcess   *os.Process
}

func (obj *SyncFtp) Init() {
	obj.syncFtpServer = nil
	obj.dependProcess = nil

	obj.allRemoteFolder = make(map[string]bool, 0)
	obj.syncFileChannel = make(chan *SyncFileInfo, 1000)
	obj.syncStopChannel = make(chan bool, 0)
	obj.activeDeadline = time.Now().Add(time.Second * DEPEND_PROCESS_TIMEOUT_SECONDS)

	go func() {
		checkInterval := time.NewTicker(time.Second * CHECK_FTP_CONNECTION_STATE_INTERVAL_SECONDS)
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
				obj.tryCloseConnectedFtpServer()
			case syncFile := <-obj.syncFileChannel:
				Logger.Printf("got sync channel file %v", syncFile)
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
				Logger.Printf("got sync channel file %v", syncFile)
				obj.Put(syncFile.LocalFile, syncFile.RemoteFile, syncFile.NumberTimes)
			default:
				break F
			}
		}

		obj.syncStopChannel <- true
	}()
}

func (obj *SyncFtp) doDependReadyAction() {
	if len(GConfig.DependCommand) == 0 {
		return
	}

	if obj.dependProcess != nil {
		_, err := os.FindProcess(obj.dependProcess.Pid)
		if err != nil {
			Logger.Print(err)
			obj.dependProcess = nil
		} else {
			Logger.Printf("depend process pid=%d already exists", obj.dependProcess.Pid)
			return
		}
	}

	command := exec.Command(GConfig.DependCommand)
	stderr, err := command.StderrPipe()
	if err == nil {
		go io.Copy(NewLogWriter("depend_process_stderr"), stderr)
	}

	stdout, err := command.StdoutPipe()
	if err == nil {
		go io.Copy(NewLogWriter("depend_process_stdout"), stdout)
	}

	err = command.Start()
	if err != nil {
		Logger.Print(err)
		return
	}

	p, err := os.FindProcess(command.Process.Pid)
	if err != nil {
		Logger.Print(err)
		return
	}

	obj.dependProcess = p

	go func() {
		pid := obj.dependProcess.Pid
		Logger.Printf("wait depend process pid=%d", pid)
		obj.dependProcess.Wait()
		obj.dependProcess = nil
		obj.syncFtpServer = nil
		Logger.Printf("wait depend process pid=%d success", pid)
	}()

	Logger.Printf("start depend process success pid=%d", obj.dependProcess.Pid)
}

func (obj *SyncFtp) doDependClearAction() {
	if obj.dependProcess == nil {
		return
	}

	pid := obj.dependProcess.Pid
	err := obj.dependProcess.Signal(syscall.SIGTERM)
	if err != nil {
		Logger.Print(err)
	}
	obj.dependProcess = nil

	Logger.Printf("kill depent process pid=%d", pid)
}

func (obj *SyncFtp) closeConnectedFtpServer() {
	if obj.syncFtpServer == nil {
		return
	}

	obj.syncFtpServer.Logout()
	obj.syncFtpServer.Quit()
	obj.syncFtpServer = nil
}

func (obj *SyncFtp) ftpServerStateIsActive() bool {
	if obj.syncFtpServer != nil {
		err := obj.syncFtpServer.NoOp()
		if err == nil {
			obj.activeDeadline = time.Now().Add(time.Second * DEPEND_PROCESS_TIMEOUT_SECONDS)
			Logger.Printf("ftp server deadline %s", obj.activeDeadline.Format(time.RFC3339))
			return true
		} else {
			Logger.Print(err)
			obj.closeConnectedFtpServer()
		}
	}

	obj.doDependReadyAction()

	loop := 0
	for loop < TRY_CONNECT_FTP_SERVER_MAX_NUM {
		loop = loop + 1
		fs, err := ftp.DialTimeout(GConfig.FtpServerAddress, time.Second*5)
		if err != nil {
			Logger.Printf("connect ftp server %s failed %v", GConfig.FtpServerAddress, err)
			time.Sleep(time.Second * 2)
			continue
		}

		fs.SetOpsTimeout(time.Second * SYNC_TO_FTP_READ_WRITE_TIMEOUT_SECONDS)

		err = fs.Login(GConfig.FtpServerUser, GConfig.FtpServerPassword)
		if err != nil {
			Logger.Printf("login ftp server %s failed %v", GConfig.FtpServerAddress, err)
			Logger.Println(err)
			return false
		}

		obj.syncFtpServer = fs
		obj.activeDeadline = time.Now().Add(time.Second * DEPEND_PROCESS_TIMEOUT_SECONDS)

		Logger.Printf("connect ftp server %s success deadline %s", GConfig.FtpServerAddress, obj.activeDeadline.Format(time.RFC3339))
		return true
	}

	Logger.Printf("connect ftp server %s failed", GConfig.FtpServerAddress)
	return false
}

func (obj *SyncFtp) tryCloseConnectedFtpServer() {
	obj.Lock()
	defer obj.Unlock()

	if obj.syncFtpServer != nil && obj.activeDeadline.Before(time.Now()) {
		Logger.Printf("overflow max timeout to disconnect ftp server")

		obj.closeConnectedFtpServer()
		obj.doDependClearAction()

		obj.allRemoteFolder = make(map[string]bool, 0)
	} else {
		allRemoteFolders := []string{}
		for k, _ := range obj.allRemoteFolder {
			allRemoteFolders = append(allRemoteFolders, k)
		}
		sort.Strings(allRemoteFolders)
		Logger.Printf("current cached ftp server folders %v", allRemoteFolders)
	}
}

func (obj *SyncFtp) Refresh() bool {
	obj.Lock()
	defer obj.Unlock()

	return obj.ftpServerStateIsActive()
}

func (obj *SyncFtp) Put(localFile, remoteFile string, numberTimes int) bool {
	obj.Lock()
	defer obj.Unlock()

	Logger.Printf("ready to put %s to %s success", localFile, remoteFile)

	reader, err := os.Open(localFile)
	if err != nil {
		Logger.Printf("open %s failed %v", localFile, err)
		return false
	}
	defer reader.Close()

	if obj.ftpServerStateIsActive() == false {
		Logger.Printf("sync %s to %s failed can't connected ftp server", localFile, remoteFile)
		obj.Async(localFile, remoteFile, numberTimes, SYNC_TO_FTP_AGAINT_LATER_SECONDS)
		return false
	}

	tmpRemoteFile := remoteFile

	if strings.HasPrefix(tmpRemoteFile, "/") {
		tmpRemoteFile = tmpRemoteFile[1:]
	}

	if len(tmpRemoteFile) > 0 && strings.HasSuffix(tmpRemoteFile, "/") {
		tmpRemoteFile = tmpRemoteFile[:len(tmpRemoteFile)-1]
	}

	if len(tmpRemoteFile) == 0 {
		Logger.Printf("sync remote file %s failed", remoteFile)
		return false
	}

	position := strings.LastIndex(tmpRemoteFile, "/")
	if position >= 0 {
		remoteFileFolder := tmpRemoteFile[:position]
		if _, ok := obj.allRemoteFolder[remoteFileFolder]; ok == false || numberTimes > 1 {
			parentPath := path.Join("/", remoteFileFolder)
			err := obj.syncFtpServer.ChangeDir(parentPath)
			if err == nil {
				obj.allRemoteFolder[parentPath] = true
			} else {
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
							if strings.Contains(err.Error(), "File exists") {
								obj.allRemoteFolder[tempRemoteFileFolder] = true
							}
							Logger.Printf("mkdir %s failed %v", tempRemoteFileFolder, err)
						}
					}
				}
			}
		}
	}

	Logger.Printf("start sync %s to %s", localFile, remoteFile)

	err = obj.syncFtpServer.Stor(remoteFile, reader)
	if err != nil {
		Logger.Printf("sync %s to %s failed %v", localFile, remoteFile, err)
		obj.Async(localFile, remoteFile, numberTimes+1, 0)
		return false
	}

	Logger.Printf("sync %s to %s success", localFile, remoteFile)

	return true
}

func (obj *SyncFtp) Async(localFile, remoteFile string, numberTimes int, waitSeconds int) bool {
	if numberTimes > SYNC_TO_FTP_MAX_NUM {
		Logger.Printf("sync %s to %s overflow max num", localFile, remoteFile)
		return false
	}

	go func() {
		if waitSeconds > 0 {
			time.Sleep(time.Duration(waitSeconds) * time.Second)
		}
		obj.syncFileChannel <- NewSyncFileInfo(localFile, remoteFile, numberTimes)
	}()

	return true
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
	<-obj.syncStopChannel
	obj.closeConnectedFtpServer()
	obj.doDependClearAction()
	Logger.Print("syncFtp stopped")
}

var GSyncFtp = &SyncFtp{}
