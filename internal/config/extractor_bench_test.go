package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"testing"
	"testing/fstest"
	"time"
)

const benchmarkFileCount = 300
const benchmarkPayloadSize = 1024

func BenchmarkExtractorCold(b *testing.B) {
	sourceFS, manifest := buildBenchmarkFS(benchmarkFileCount, benchmarkPayloadSize)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		destDir := b.TempDir()
		extractor := Extractor{BackupLimit: 1}
		if _, err := extractor.ExtractWithStats(sourceFS, destDir, manifest); err != nil {
			b.Fatalf("extract failed: %v", err)
		}
	}
}

func BenchmarkExtractorWarm(b *testing.B) {
	sourceFS, manifest := buildBenchmarkFS(benchmarkFileCount, benchmarkPayloadSize)
	destDir := b.TempDir()
	extractor := Extractor{BackupLimit: 1}
	if _, err := extractor.ExtractWithStats(sourceFS, destDir, manifest); err != nil {
		b.Fatalf("extract failed: %v", err)
	}
	referenceTime := time.Now().UTC()
	warmExtractor := Extractor{BackupLimit: 1, LastUpdated: referenceTime}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := warmExtractor.ExtractWithStats(sourceFS, destDir, manifest); err != nil {
			b.Fatalf("extract failed: %v", err)
		}
	}
}

func buildBenchmarkFS(fileCount, payloadSize int) (fstest.MapFS, map[string]string) {
	fsys := make(fstest.MapFS, fileCount)
	manifest := make(map[string]string, fileCount)
	for i := 0; i < fileCount; i++ {
		relPath := fmt.Sprintf("agents/agent-%04d.json", i)
		content := []byte(fmt.Sprintf(`{"name":"Agent %04d","shell":"/bin/bash"}`, i))
		if payloadSize > len(content) {
			content = append(content, bytes.Repeat([]byte("x"), payloadSize-len(content))...)
		}
		sum := sha256.Sum256(content)
		manifest[relPath] = hex.EncodeToString(sum[:])
		fsys[path.Join("config", relPath)] = &fstest.MapFile{Data: content}
	}
	return fsys, manifest
}
