// +build darwin dragonfly freebsd nacl netbsd openbsd solaris windows
package ops

import (
	"errors"
	"log"
	"os"
)

func notSupported()(int, error){
	log.Fatalln("xattrs not supported")
	return 0, errors.New("Not supported")
}

func getxattr(path string, name string, data []byte) (int, error) {
	return notSupported()
}

func lgetxattr(path string, name string, data []byte) (int, error) {
	return notSupported()
}

func fgetxattr(f *os.File, name string, data []byte) (int, error) {
	return notSupported()
}

func setxattr(path string, name string, data []byte, flags int) error {
	_,e := notSupported()
	return e
}

func lsetxattr(path string, name string, data []byte, flags int) error {
	_,e := notSupported()
	return e
}

func fsetxattr(f *os.File, name string, data []byte, flags int) error {
	_,e := notSupported()
	return e
}

func removexattr(path string, name string) error {
	_,e := notSupported()
	return e
}

func lremovexattr(path string, name string) error {
	_,e := notSupported()
	return e
}

func fremovexattr(f *os.File, name string) error {
	_,e := notSupported()
	return e
}

func listxattr(path string, data []byte) (int, error) {
	return notSupported()
}

func llistxattr(path string, data []byte) (int, error) {
	return notSupported()
}

func flistxattr(f *os.File, data []byte) (int, error) {
	return notSupported()
}
