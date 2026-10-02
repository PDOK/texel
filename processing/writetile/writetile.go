package writetile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/pdok/texel/config"
	"github.com/pdok/texel/processing"
)

type TileWriter struct {
	targets []processing.MVTTarget
}

func NewTileWriter(conf config.TomlConfig) (*TileWriter, error) {
	var targets []processing.MVTTarget
	if conf.Cache.Azure != nil {
		at, err := NewMVTAzureTarget(conf)
		if err != nil {
			return nil, err
		}
		targets = append(targets, at)
	}

	if conf.Cache.File != nil {
		ft, err := NewMVTFileTarget(conf)
		if err != nil {
			return nil, err
		}
		targets = append(targets, ft)
	}

	if len(targets) == 0 {
		return nil, errors.New("no configured targets for texel mvt")
	}

	return &TileWriter{
		targets: targets,
	}, nil
}

func (t *TileWriter) WriteTile(x, y, z uint, data []byte) error {
	var err error
	for _, target := range t.targets {
		err = target.WriteTile(x, y, z, data)
		if err != nil {
			return err
		}
	}

	return nil
}

////////////////////
// MVTAzureTarget //
////////////////////

const (
	mvtBlobContentType     = "application/vnd.mapbox-vector-tile"
	mvtBlobContentEncoding = "gzip"
)

type blobUploader interface {
	UploadBuffer(
		ctx context.Context,
		containerName string,
		blobName string,
		buffer []byte,
		options *azblob.UploadBufferOptions,
	) (azblob.UploadBufferResponse, error)
}

// MVTAzureTarget uploads each built MVT tile to Azure Blob Storage before
// WriteTile returns.
type MVTAzureTarget struct {
	client    blobUploader
	container string
	prefix    string
}

func NewMVTAzureTarget(conf config.TomlConfig) (*MVTAzureTarget, error) {
	connString := conf.Cache.Azure.ConnectionString
	container := conf.Cache.Azure.Container

	client, err := newMVTBlobClient(connString, container)
	if err != nil {
		return nil, err
	}

	return &MVTAzureTarget{
		client:    client,
		container: container,
		prefix:    buildPrefix(conf),
	}, nil
}

func newMVTBlobClient(connectionString, container string) (blobUploader, error) {
	if connectionString == "" {
		return nil, errors.New("azure blob storage connection string is required")
	}
	if container == "" {
		return nil, errors.New("azure blob storage container is required")
	}
	client, err := azblob.NewClientFromConnectionString(connectionString, nil)
	if err != nil {
		return nil, errors.New("creating azure blob storage client: invalid connection string")
	}
	return client, nil
}

func buildPrefix(conf config.TomlConfig) string {
	return path.Join(
		strings.Trim(conf.Cache.Azure.PrefixKey, "/"),
		conf.Tileset[0].Name,
	)
}

// WriteTile uploads one tile before returning.
func (t *MVTAzureTarget) WriteTile(x, y, z uint, data []byte) error {
	name := mvtBlobName(t.prefix, x, y, z)
	_, err := t.client.UploadBuffer(context.Background(), t.container, name, data, &azblob.UploadBufferOptions{
		HTTPHeaders: mvtBlobHTTPHeaders(),
	})
	if err != nil {
		return fmt.Errorf("uploading blob %q: %w", name, err)
	}
	return nil
}

func mvtBlobName(prefix string, x, y, z uint) string {
	return path.Join(
		prefix,
		strconv.FormatUint(uint64(z), 10),
		strconv.FormatUint(uint64(x), 10),
		strconv.FormatUint(uint64(y), 10)+".pbf",
	)
}

func mvtBlobHTTPHeaders() *blob.HTTPHeaders {
	return &blob.HTTPHeaders{
		BlobContentType:     new(mvtBlobContentType),
		BlobContentEncoding: new(mvtBlobContentEncoding),
	}
}

///////////////////
// MVTFileTarget //
///////////////////

// MVTFileTarget writes built MVT tiles to <OutDir>/<tileX>/<tileY>.pbf
type MVTFileTarget struct {
	OutDir string
}

func NewMVTFileTarget(conf config.TomlConfig) (*MVTFileTarget, error) {
	base := conf.Cache.File.Base
	outDir := filepath.Join(base, conf.Tileset[0].Name)

	err := os.MkdirAll(outDir, 0o775)
	if err != nil {
		return nil, err
	}

	return &MVTFileTarget{
		OutDir: outDir,
	}, nil
}

// WriteTile writes one tile's serialized bytes to <OutDir>/<tileX>/<tileY>.mvt,
// creating directories as needed.
func (t *MVTFileTarget) WriteTile(x, y, z uint, data []byte) error {
	dir := filepath.Join(t.OutDir, strconv.FormatUint(uint64(z), 10), strconv.FormatUint(uint64(x), 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating tile directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, strconv.FormatUint(uint64(y), 10)+".pbf")
	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // G306 tile output does not need restrictive permissions
		return fmt.Errorf("writing tile file %s: %w", path, err)
	}
	return nil
}
