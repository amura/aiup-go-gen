package utils

import (
    "os"
    "path/filepath"
    "io/ioutil"
)

func CreateTempDir() string {
    tmp, _ := ioutil.TempDir("", "exec")
    return tmp
}

func WriteTempFile(dir, filename, content string) error {
    path := filepath.Join(dir, filename)
    return ioutil.WriteFile(path, []byte(content), 0644)
}

func RemoveTempDir(dir string) {
    os.RemoveAll(dir)
}