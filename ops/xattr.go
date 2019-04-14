package ops

func Getxattr(path string, name string, data []byte) (int, error) {
	return getxattr(path, name, data)
}

func Setxattr(path string, name string, data []byte, flags int) error {
	return setxattr(path, name, data, flags)
}

func Listxattr(path string, data []byte) (int, error) {
	return listxattr(path, data)
}

func Removexattr(path string, name string) error {
	return removexattr(path, name)
}
