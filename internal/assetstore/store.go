package assetstore

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaxAssetBytes       = 10 << 20
	MaxOwnerAssetBytes  = 512 << 20
	MaxOwnerAssets      = 5000
	MaxTotalAssetBytes  = 2 << 30
	MaxTotalAssets      = 20_000
	MaxImportAssetBytes = 64 << 20
	maxImageSide        = 8192
	maxImagePixels      = 40_000_000
)

type Blob struct {
	SHA256   string
	MIMEType string
	Size     int64
	Width    int
	Height   int
}

type Store struct {
	rootPath string
	root     *os.Root
}

type StagedBlob struct {
	Blob
	store         *Store
	temporaryName string
	finalName     string
	published     bool
	created       bool
}

func New(rootPath string) (*Store, error) {
	absolute, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolve asset directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o750); err != nil {
		return nil, fmt.Errorf("create asset directory: %w", err)
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, fmt.Errorf("open asset directory: %w", err)
	}
	entries, err := os.ReadDir(absolute)
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("inspect asset directory: %w", err)
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), ".upload-") {
			if err := root.Remove(entry.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
				_ = root.Close()
				return nil, fmt.Errorf("remove interrupted staged asset: %w", err)
			}
		}
	}
	return &Store{rootPath: absolute, root: root}, nil
}

func (store *Store) Close() error {
	return store.root.Close()
}

func (store *Store) Save(source io.Reader) (Blob, error) {
	staged, err := store.Stage(source)
	if err != nil {
		return Blob{}, err
	}
	defer staged.Abort()
	if err := staged.Publish(); err != nil {
		return Blob{}, err
	}
	return staged.Blob, nil
}

func (store *Store) Stage(source io.Reader) (*StagedBlob, error) {
	temporaryName, err := randomTemporaryName()
	if err != nil {
		return nil, err
	}
	temporary, err := store.root.OpenFile(temporaryName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create temporary asset: %w", err)
	}
	keepTemporary := false
	defer func() {
		_ = temporary.Close()
		if !keepTemporary {
			_ = store.root.Remove(temporaryName)
		}
	}()

	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(source, MaxAssetBytes+1))
	if err != nil {
		return nil, fmt.Errorf("store uploaded asset: %w", err)
	}
	if size > MaxAssetBytes {
		return nil, fmt.Errorf("asset exceeds %d bytes", MaxAssetBytes)
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind uploaded asset: %w", err)
	}
	header := make([]byte, 512)
	headerSize, err := io.ReadFull(temporary, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("read uploaded asset header: %w", err)
	}
	mimeType := http.DetectContentType(header[:headerSize])
	extension, ok := extensionForMIME(mimeType)
	if !ok {
		return nil, fmt.Errorf("unsupported asset type")
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind uploaded image: %w", err)
	}
	configuration, format, err := image.DecodeConfig(temporary)
	if err != nil || "image/"+format != mimeType {
		return nil, fmt.Errorf("uploaded asset is not a valid %s image", mimeType)
	}
	if configuration.Width < 1 || configuration.Height < 1 || configuration.Width > maxImageSide || configuration.Height > maxImageSide || int64(configuration.Width)*int64(configuration.Height) > maxImagePixels {
		return nil, fmt.Errorf("uploaded image dimensions are not allowed")
	}

	digest := hex.EncodeToString(hash.Sum(nil))
	blob := Blob{SHA256: digest, MIMEType: mimeType, Size: size, Width: configuration.Width, Height: configuration.Height}
	if err := temporary.Chmod(0o640); err != nil {
		return nil, fmt.Errorf("set staged asset permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return nil, fmt.Errorf("close staged asset: %w", err)
	}
	keepTemporary = true
	return &StagedBlob{
		Blob: blob, store: store, temporaryName: temporaryName,
		finalName: digest[:2] + "/" + digest + extension,
	}, nil
}

func (staged *StagedBlob) Publish() error {
	if staged.published {
		return nil
	}
	shard := staged.SHA256[:2]
	if err := staged.store.root.Mkdir(shard, 0o750); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create asset shard: %w", err)
	}
	if info, err := staged.store.root.Stat(staged.finalName); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("asset destination is not a regular file")
		}
		if err := staged.store.root.Remove(staged.temporaryName); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove duplicate staged asset: %w", err)
		}
		staged.published = true
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect asset destination: %w", err)
	}
	if err := staged.store.root.Rename(staged.temporaryName, staged.finalName); err != nil {
		if _, statErr := staged.store.root.Stat(staged.finalName); statErr == nil {
			_ = staged.store.root.Remove(staged.temporaryName)
			staged.published = true
			return nil
		}
		return fmt.Errorf("publish asset: %w", err)
	}
	staged.published = true
	staged.created = true
	return nil
}

func (staged *StagedBlob) Abort() {
	if staged == nil || staged.published {
		return
	}
	_ = staged.store.root.Remove(staged.temporaryName)
}

func (staged *StagedBlob) RollbackPublished() {
	if staged == nil || !staged.published || !staged.created {
		return
	}
	_ = staged.store.root.Remove(staged.finalName)
	staged.published = false
	staged.created = false
}

func (store *Store) Reconcile(referenced []Blob) error {
	allowed := make(map[string]struct{}, len(referenced))
	for _, blob := range referenced {
		if !validSHA256(blob.SHA256) {
			return fmt.Errorf("reconcile asset with invalid digest")
		}
		extension, ok := extensionForMIME(blob.MIMEType)
		if !ok {
			return fmt.Errorf("reconcile asset with invalid MIME type")
		}
		allowed[blob.SHA256[:2]+"/"+blob.SHA256+extension] = struct{}{}
	}
	shards, err := os.ReadDir(store.rootPath)
	if err != nil {
		return fmt.Errorf("read asset shards: %w", err)
	}
	for _, shard := range shards {
		if !shard.IsDir() || len(shard.Name()) != 2 {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(store.rootPath, shard.Name()))
		if err != nil {
			return fmt.Errorf("read asset shard: %w", err)
		}
		for _, entry := range entries {
			if !entry.Type().IsRegular() {
				continue
			}
			relative := shard.Name() + "/" + entry.Name()
			if _, exists := allowed[relative]; exists {
				continue
			}
			if err := store.root.Remove(relative); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove unreferenced asset: %w", err)
			}
		}
	}
	return nil
}

func (store *Store) Open(blob Blob) (*os.File, error) {
	if !validSHA256(blob.SHA256) {
		return nil, fmt.Errorf("invalid asset digest")
	}
	extension, ok := extensionForMIME(blob.MIMEType)
	if !ok {
		return nil, fmt.Errorf("invalid asset MIME type")
	}
	file, err := store.root.Open(blob.SHA256[:2] + "/" + blob.SHA256 + extension)
	if err != nil {
		return nil, fmt.Errorf("open asset: %w", err)
	}
	return file, nil
}

func (store *Store) Remove(blob Blob) error {
	if !validSHA256(blob.SHA256) {
		return fmt.Errorf("invalid asset digest")
	}
	extension, ok := extensionForMIME(blob.MIMEType)
	if !ok {
		return fmt.Errorf("invalid asset MIME type")
	}
	err := store.root.Remove(blob.SHA256[:2] + "/" + blob.SHA256 + extension)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func extensionForMIME(mimeType string) (string, bool) {
	switch mimeType {
	case "image/png":
		return ".png", true
	case "image/jpeg":
		return ".jpg", true
	default:
		return "", false
	}
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func randomTemporaryName() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate temporary asset name: %w", err)
	}
	return ".upload-" + hex.EncodeToString(bytes), nil
}
