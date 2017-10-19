package cache

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"time"
)

type Cache struct {
}

func (c *Cache) ReadCache(url string) ([]byte, error) {
	path, err := c.buildPath(url)
	if err != nil {
		return nil, err
	}

	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func (c *Cache) WriteCache(url string, content []byte) {
	cachePath, err := c.buildPath(url)
	if err != nil {
		return
	}
	os.MkdirAll(path.Dir(cachePath), 0777)

	err = ioutil.WriteFile(cachePath, content, 0777)
	if err != nil {
		fmt.Printf("Writing failed\n")
		fmt.Printf("%s\n", err)
	}
}

func (c *Cache) buildPath(url string) (string, error) {
	curUser, err := user.Current()
	if err != nil {
		return "", err
	}
	home := curUser.HomeDir
	path := filepath.Join(home, ".config/bahn/cache/", url)
	return path, nil
}

func (c *Cache) ClearCache() {
	cachDir, err := c.buildPath("")
	if err != nil {
		fmt.Printf("Cleaning cache failed\n")
		fmt.Printf("%s\n", err)
	}
	yesterday := time.Now().Add(-24 * time.Hour)

	filepath.Walk(cachDir, func(curPath string, info os.FileInfo, err error) error {
		if path.Base(curPath) == "cache" {
			return nil
		}
		if info.ModTime().Before(yesterday) {
			err := os.Remove(curPath)
			if err != nil {
				fmt.Printf("Could not remove %s\n", curPath)
				fmt.Printf("%s\n", err)
			}
		}
		return nil
	})

}
