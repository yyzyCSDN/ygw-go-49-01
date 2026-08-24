package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/cespare/xxhash/v2"
)

// DigestOf computes the content address of a blob: "sha256:<hex>".
func DigestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Shard maps a digest to one of buckets shards using xxhash. The registry
// keeps separate shard tables so that large repositories can spread their
// blob index over multiple maps; the exact shard number is stable for a
// digest and is used by dedup lookups and by GC scans.
func Shard(digest string, buckets int) int {
	if buckets <= 0 {
		buckets = 1
	}
	return int(xxhash.Sum64String(digest) % uint64(buckets))
}

// ShardLabel returns a short stable label for a digest shard.
func ShardLabel(digest string, buckets int) string {
	return fmt.Sprintf("shard-%02d", Shard(digest, buckets))
}
