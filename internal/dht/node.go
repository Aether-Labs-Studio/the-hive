package dht

import (
	"bytes"
	"crypto/sha1"
)

// NodeID is a 160-bit (20-byte) identifier.
type NodeID [20]byte

// NewNodeID generates a NodeID by hashing the input string using SHA-1.
func NewNodeID(data string) NodeID {
	return sha1.Sum([]byte(data))
}

// XOR calculates the distance between two NodeIDs.
// It returns a new NodeID representing the bitwise XOR result.
func (n NodeID) XOR(other NodeID) NodeID {
	var result NodeID
	for i := 0; i < 20; i++ {
		result[i] = n[i] ^ other[i]
	}
	return result
}

// Less compares two NodeIDs bit by bit.
// It returns true if n is mathematically less than other.
func (n NodeID) Less(other NodeID) bool {
	return bytes.Compare(n[:], other[:]) < 0
}
