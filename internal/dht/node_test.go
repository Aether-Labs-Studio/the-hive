package dht

import (
	"bytes"
	"crypto/sha1"
	"testing"
)

func TestNewNodeID(t *testing.T) {
	data := "test-node-data"
	id1 := NewNodeID(data)
	id2 := NewNodeID(data)

	// Verify consistent generation
	if !bytes.Equal(id1[:], id2[:]) {
		t.Errorf("NewNodeID is inconsistent: got %x and %x for same data", id1, id2)
	}

	// Verify SHA-1 hash integrity
	expected := sha1.Sum([]byte(data))
	if !bytes.Equal(id1[:], expected[:]) {
		t.Errorf("NewNodeID does not match expected SHA-1: got %x, want %x", id1, expected)
	}

	// Verify length (20 bytes for SHA-1)
	if len(id1) != 20 {
		t.Errorf("NodeID length is incorrect: got %d, want 20", len(id1))
	}
}

func TestXORMetric(t *testing.T) {
	idA := NewNodeID("node-a")
	idB := NewNodeID("node-b")

	// 1. Distance to self is zero
	distAA := idA.XOR(idA)
	zero := NodeID{}
	if !bytes.Equal(distAA[:], zero[:]) {
		t.Errorf("XOR distance to self is not zero: %x", distAA)
	}

	// 2. Symmetry: Dist(A, B) == Dist(B, A)
	distAB := idA.XOR(idB)
	distBA := idB.XOR(idA)
	if !bytes.Equal(distAB[:], distBA[:]) {
		t.Errorf("XOR distance is not symmetric: Dist(A,B)=%x, Dist(B,A)=%x", distAB, distBA)
	}

	// 3. Triangle inequality property (A XOR B) XOR (B XOR C) == A XOR C
	idC := NewNodeID("node-c")
	distAC := idA.XOR(idC)
	xorOfDists := distAB.XOR(idB.XOR(idC))
	if !bytes.Equal(distAC[:], xorOfDists[:]) {
		t.Errorf("XOR distance property failed: (A^B)^(B^C) != A^C")
	}
}

func TestNodeIDLess(t *testing.T) {
	// Creating predictable IDs for comparison
	// id1: 00...01
	// id2: 00...02
	id1 := NodeID{}
	id1[19] = 1
	id2 := NodeID{}
	id2[19] = 2

	if !id1.Less(id2) {
		t.Errorf("Less comparison failed: %x should be less than %x", id1, id2)
	}

	if id2.Less(id1) {
		t.Errorf("Less comparison failed: %x should not be less than %x", id2, id1)
	}

	if id1.Less(id1) {
		t.Errorf("Less comparison failed: %x should not be less than itself", id1)
	}
}
