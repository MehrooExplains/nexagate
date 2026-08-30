package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func BackupCreate(output, configPath, statePath, generatedDir, password string) error {
	var raw strings.Builder
	files := []string{configPath, statePath}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		fmt.Fprintf(&raw, "FILE:%s\nSIZE:%d\n", filepath.Base(path), len(data))
		raw.Write(data)
		raw.WriteString("\nEND_FILE\n")
	}
	if entries, err := os.ReadDir(generatedDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				data, er := os.ReadFile(filepath.Join(generatedDir, e.Name()))
				if er == nil {
					fmt.Fprintf(&raw, "FILE:generated/%s\nSIZE:%d\n", e.Name(), len(data))
					raw.Write(data)
					raw.WriteString("\nEND_FILE\n")
				}
			}
		}
	}
	data := []byte(raw.String())
	if password != "" {
		var err error
		data, err = encryptBackup(data, password)
		if err != nil {
			return err
		}
	}
	return os.WriteFile(output, data, 0600)
}

func BackupRestore(input, destination, password string) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	if password != "" {
		data, err = decryptBackup(data, password)
		if err != nil {
			return err
		}
	}
	return os.WriteFile(destination, data, 0600)
}

func BackupList(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	fmt.Printf("%s\t%d bytes\n", path, info.Size())
	return nil
}

func backupKey(password string) []byte { sum := sha256.Sum256([]byte(password)); return sum[:] }
func encryptBackup(data []byte, password string) ([]byte, error) {
	b, err := aes.NewCipher(backupKey(password))
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	n := make([]byte, g.NonceSize())
	if _, err = io.ReadFull(rand.Reader, n); err != nil {
		return nil, err
	}
	return append([]byte("NEXABKP1"), append(n, g.Seal(nil, n, data, nil)...)...), nil
}
func decryptBackup(data []byte, password string) ([]byte, error) {
	if len(data) < 8 || string(data[:8]) != "NEXABKP1" {
		return data, nil
	}
	b, err := aes.NewCipher(backupKey(password))
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	n := data[8 : 8+g.NonceSize()]
	out, err := g.Open(nil, n, data[8+g.NonceSize():], nil)
	if err != nil {
		return nil, errors.New("invalid backup password or corrupted backup")
	}
	return out, nil
}
