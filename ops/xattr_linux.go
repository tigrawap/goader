// +build linux
package ops

import "golang.org/x/sys/unix"

func getxattr(path string, name string, data []byte) (int, error) {
	return unix.Getxattr(path, name, data)
}

func lgetxattr(path string, name string, data []byte) (int, error) {
	return unix.Lgetxattr(path, name, data)
}

//func fgetxattr(f *os.File, name string, data []byte) (int, error) {
//	return unix.Fgetxattr(int(f.Fd()), name, data)
//}

func setxattr(path string, name string, data []byte, flags int) error {
	return unix.Setxattr(path, name, data, flags)
}

func lsetxattr(path string, name string, data []byte, flags int) error {
	return unix.Lsetxattr(path, name, data, flags)
}

//func fsetxattr(f *os.File, name string, data []byte, flags int) error {
//	return unix.Fsetxattr(int(f.Fd()), name, data, flags)
//}

func removexattr(path string, name string) error {
	return unix.Removexattr(path, name)
}

func lremovexattr(path string, name string) error {
	return unix.Lremovexattr(path, name)
}

//func fremovexattr(f *os.File, name string) error {
//	return unix.Fremovexattr(int(f.Fd()), name)
//}

func listxattr(path string, data []byte) (int, error) {
	return unix.Listxattr(path, data)
}

func llistxattr(path string, data []byte) (int, error) {
	return unix.Llistxattr(path, data)
}

//func flistxattr(f *os.File, data []byte) (int, error) {
//	return unix.Flistxattr(int(f.Fd()), data)
//}
