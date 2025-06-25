// Copyright DataStax, Inc.
// Please see the included license file for details.

package utils

import (
	"crypto/sha256"
	"encoding/base64"
	"hash"

	"github.com/davecgh/go-spew/spew"
)

type Annotated interface {
	GetAnnotations() map[string]string
	SetAnnotations(annotations map[string]string)
}

const ResourceHashAnnotationKey = "cassandra.datastax.com/resource-hash"

func ResourcesHaveSameHash(r1, r2 Annotated) bool {
	a1 := r1.GetAnnotations()
	a2 := r2.GetAnnotations()
	if a1 == nil || a2 == nil {
		return false
	}
	return a1[ResourceHashAnnotationKey] == a2[ResourceHashAnnotationKey]
}

func AddHashAnnotation(r Annotated) {
	hash := deepHashString(r)
	m := r.GetAnnotations()
	if m == nil {
		m = map[string]string{}
	}
	m[ResourceHashAnnotationKey] = hash
	r.SetAnnotations(m)
}

func deepHashString(obj interface{}) string {
	hasher := sha256.New()
	DeepHashObject(hasher, obj)
	hashBytes := hasher.Sum([]byte{})
	b64Hash := base64.StdEncoding.EncodeToString(hashBytes)
	return b64Hash
}

// DeepHashObject writes specified object to hash using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
func DeepHashObject(hasher hash.Hash, objectToWrite interface{}) {
	hasher.Reset()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	if _, err := printer.Fprintf(hasher, "%#v", objectToWrite); err != nil {
		// We're ignoring errors for backward compatibility
		return
	}
}
