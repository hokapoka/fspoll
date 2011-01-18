package fspoll

import (
	"os"
	"fmt"
	"time"
	"sync"
	"bytes"
	"io"
)

const second = 1e9

type filePolledDetails struct{
	name string
	modT int64
	stat *os.FileInfo
	modified bool
	buf *bytes.Buffer
	lastErr os.Error
	mu sync.RWMutex
}

func newFilePolledDetails(name string) (*filePolledDetails, os.Error){
	d := &filePolledDetails{
		name:name,
	}
	err := d.update()
	if err != nil{
		return nil, err
	}
	return d, nil
}

func(self *filePolledDetails) update() os.Error{
	stat, err := os.Stat(self.name)
	if err != nil {
		self.mu.Lock()
		defer self.mu.Unlock()

		self.lastErr = err
		return err
	}

	if self.modT < stat.Mtime_ns {

		self.mu.Lock()
		defer self.mu.Unlock()

		time.Sleep(second)

		f, err := os.Open(self.name, os.O_RDONLY, 0644)
		if err != nil {
			self.lastErr = err
			return err
		}
		defer f.Close()

		var b bytes.Buffer
		_, err = io.Copy(&b, f)
		if err != nil {
			self.lastErr = err
			return err
		}

		if b.String() == "" { // sshfs & vim effects write
			f2, err := os.Open(self.name, os.O_RDONLY, 0644)
			defer f2.Close()
			time.Sleep(second * 5)
			_, err = io.Copy(&b, f2)
			if err != nil {
				self.lastErr = err
				return err
			}
		}

		self.buf		= &b
		self.modT		= stat.Mtime_ns
		self.modified	= true
		self.lastErr	= nil

	}

	return nil
}

type FilePoller struct{
	files map[string]*filePolledDetails
	mu sync.RWMutex
	timeout int64
}

func NewFilePoller(timeout int64) (*FilePoller) {

	if timeout <= 0 {
		timeout = 1 // 1e9 * 0 == 0
	}


	p := &FilePoller{
		files:map[string]*filePolledDetails{},
	}
	go p.start()

	return p
}

func(self *FilePoller) start(){
	for{
		self.pollFiles()
		time.Sleep(second * self.timeout)
	}
}

func(self *FilePoller) pollFiles(){
	self.mu.RLock()
	defer self.mu.RUnlock()

	for _, v := range self.files {
		_ = v.update()
	}
}

func(self *FilePoller) Add(name string) (os.Error){

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.files[name] != nil {
		return os.NewError(fmt.Sprintf("File %s is already loaded in the poller", name))
	}

	f, err := newFilePolledDetails(name)
	if err != nil {
		return err
	}

	// Add to maps
	self.files[name] = f

	return nil
}

func(self *FilePoller) Get(name string) (*bytes.Buffer, bool, os.Error){

	self.mu.RLock()
	defer self.mu.RUnlock()

	if self.files[name] == nil {
		err := os.NewError(fmt.Sprintf("File %s is not loaded in the poller", name))
		return nil, false, err
	}
	f := self.files[name]
	modified := f.modified

	f.modified = false

	return f.buf, modified, f.lastErr
}

