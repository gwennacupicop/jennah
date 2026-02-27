package hashing

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/buraksezer/consistent"
)

type Member string

func (m Member) String() string {
	return string(m)
}

type hasher struct {
}

func (h hasher) Sum64(data []byte) uint64 {
	out := sha256.Sum256(data)
	return binary.BigEndian.Uint64(out[:8])
}

type Router struct {
	ring *consistent.Consistent
}

func NewRouter(members []string) *Router {
	cfg := consistent.Config{
		PartitionCount:    71,
		ReplicationFactor: 20,
		Load:              1.25,
		Hasher:            hasher{},
	}
	c := consistent.New(nil, cfg)

	for _, ip := range members {
		c.Add(Member(ip))
	}

	return &Router{ring: c}
}

// GetWorkerIP returns the worker IP for the given routing key using consistent hashing.
// The key should be unique per request to ensure even load distribution across workers.
func (r *Router) GetWorkerIP(key string) string {

	member := r.ring.LocateKey([]byte(key))
	if member == nil {
		return ""
	}
	return member.String()
}
