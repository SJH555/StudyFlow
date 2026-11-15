// Command syncassets stages icons and frontend output for the desktop entrypoint.
// The reminder package embeds its own source files directly.
package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func main() {
	if err := syncAssets(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func syncAssets() error {
	if err := os.RemoveAll("desktop/assets"); err != nil {
		return err
	}
	if err := copyFile("resources/Icon.png", "desktop/assets/Icon.png"); err != nil {
		return err
	}
	if err := copyFile("vue/public/app-icon.png", "desktop/assets/app-icon.png"); err != nil {
		return err
	}
	if _, err := os.Stat("vue/dist/index.html"); err == nil {
		return copyTree("vue/dist", "desktop/ui")
	} else if !os.IsNotExist(err) {
		return err
	}
	// Binding generation loads the desktop package before the first Vite build.
	return copyFile("vue/index.html", "desktop/ui/index.html")
}

func copyTree(source, target string) error {
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0755)
		}
		return copyFile(path, destination)
	})
}

func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	output, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}
