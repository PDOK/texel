package writetile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/pdok/texel/config"
	"github.com/pdok/texel/processing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTileTarget struct {
	name  string
	calls *[]tileWriteCall
	err   error
}

type tileWriteCall struct {
	name string
	x    uint
	y    uint
	z    uint
	data []byte
}

func (f fakeTileTarget) WriteTile(x, y, z uint, data []byte) error {
	*f.calls = append(*f.calls, tileWriteCall{
		name: f.name,
		x:    x,
		y:    y,
		z:    z,
		data: append([]byte(nil), data...),
	})
	return f.err
}

func TestNewTileWriter(t *testing.T) {
	tests := []struct {
		name               string
		targetConstructors []targetConstructor
		conf               config.TomlConfig
		wantErr            bool
		numConf            int
	}{
		{
			name:               "no constructors",
			targetConstructors: []targetConstructor{},
			conf:               config.TomlConfig{},
			wantErr:            true,
		},
		{
			name: "several constructors, some nil",
			targetConstructors: []targetConstructor{
				func(config.TomlConfig) (processing.MVTTarget, error) {
					return fakeTileTarget{name: "first", calls: nil}, nil
				},
				func(config.TomlConfig) (processing.MVTTarget, error) {
					return nil, nil
				},
				func(config.TomlConfig) (processing.MVTTarget, error) {
					return fakeTileTarget{name: "second", calls: nil}, nil
				},
			},
			conf:    config.TomlConfig{},
			wantErr: false,
			numConf: 2,
		},
		{
			name: "only nil constructors",
			targetConstructors: []targetConstructor{
				func(config.TomlConfig) (processing.MVTTarget, error) {
					return nil, nil
				},
				func(config.TomlConfig) (processing.MVTTarget, error) {
					return nil, nil
				},
			},
			conf:    config.TomlConfig{},
			wantErr: true,
		},
		{
			name: "error during construction",
			targetConstructors: []targetConstructor{
				func(config.TomlConfig) (processing.MVTTarget, error) {
					return nil, nil
				},
				func(config.TomlConfig) (processing.MVTTarget, error) {
					return nil, errors.New("error")
				},
			},
			conf:    config.TomlConfig{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer, err := newTileWriter(tt.conf, tt.targetConstructors)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, writer.targets, tt.numConf)
		})
	}
}

func TestBuildPrefix(t *testing.T) {
	tests := []struct {
		name      string
		prefixKey string
		want      string
	}{
		{name: "empty prefix", prefixKey: "", want: "NetherlandsRDNewQuad"},
		{name: "prefix without slashes", prefixKey: "tiles/base", want: "tiles/base/NetherlandsRDNewQuad"},
		{name: "prefix with surrounding slashes", prefixKey: "/tiles/base/", want: "tiles/base/NetherlandsRDNewQuad"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := config.TomlConfig{
				Tileset: []config.Tileset{{Name: "NetherlandsRDNewQuad"}},
				Cache: config.CacheConfig{
					Azure: &config.AzureConfig{PrefixKey: tt.prefixKey},
				},
			}
			if got := buildPrefix(conf); got != tt.want {
				t.Errorf("buildPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMvtBlobName(t *testing.T) {
	got := mvtBlobName("lv-bgt/NetherlandsRDNewQuad", 3, 4, 12)
	want := "lv-bgt/NetherlandsRDNewQuad/12/3/4.pbf"
	assert.Equal(t, want, got)
}

type fakeBlobUploader struct {
	container string
	name      string
	data      []byte
	options   *azblob.UploadBufferOptions
	err       error
}

func (f *fakeBlobUploader) UploadBuffer(
	_ context.Context,
	containerName, blobName string,
	buffer []byte,
	options *azblob.UploadBufferOptions,
) (azblob.UploadBufferResponse, error) {
	f.container = containerName
	f.name = blobName
	f.data = append([]byte(nil), buffer...)
	f.options = options
	return azblob.UploadBufferResponse{}, f.err
}

func TestMVTAzureTargetWriteTile(t *testing.T) {
	fake := &fakeBlobUploader{}
	target := &MVTAzureTarget{
		client:    fake,
		container: "public",
		prefix:    "lv-bgt/NetherlandsRDNewQuad",
	}
	data := []byte("tile bytes")

	if err := target.WriteTile(3, 4, 12, data); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "public", fake.container)
	assert.Equal(t, data, fake.data)
	assert.Equal(t, "lv-bgt/NetherlandsRDNewQuad/12/3/4.pbf", fake.name)

	assert.NotNil(t, fake.options)
	assert.NotNil(t, fake.options.HTTPHeaders)
	assert.NotNil(t, fake.options.HTTPHeaders.BlobContentType)
	assert.NotNil(t, fake.options.HTTPHeaders.BlobContentEncoding)

	assert.Equal(t, mvtBlobContentType, *fake.options.HTTPHeaders.BlobContentType)
	assert.Equal(t, mvtBlobContentEncoding, *fake.options.HTTPHeaders.BlobContentEncoding)
}

func TestMVTAzureTargetWriteTileReturnsUploadError(t *testing.T) {
	wantErr := errors.New("upload failed")
	target := &MVTAzureTarget{
		client:    &fakeBlobUploader{err: wantErr},
		container: "public",
		prefix:    "lv-bgt/NetherlandsRDNewQuad",
	}

	err := target.WriteTile(3, 4, 12, nil)
	assert.ErrorIs(t, err, wantErr)
}

func TestMVTFileTargetWritesTileUnderConfiguredBase(t *testing.T) {
	tilesetName := "NetherlandsRDNewQuad"
	data := []byte("tile bytes")
	base := t.TempDir()
	conf := config.TomlConfig{
		Tileset: []config.Tileset{{Name: tilesetName}},
		Cache: config.CacheConfig{
			File: &config.FileConfig{Base: base},
		},
	}

	target, err := NewMVTFileTarget(conf)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, tilesetName)
	assert.Equal(t, want, target.OutDir)

	err = target.WriteTile(3, 4, 12, data)
	if err != nil {
		t.Fatal(err)
	}

	tilePath := filepath.Join(base, tilesetName, "12", "3", "4.pbf")
	got, err := os.ReadFile(tilePath)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, data, got)
}
