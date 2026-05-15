// Copyright DataStax, Inc.
// Please see the included license file for details.

package utils

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_RangeInt(t *testing.T) {
	assert.Equal(t, []int{0, 2, 4, 6, 8}, RangeInt(0, 10, 2))
	assert.Equal(t, []int{0, 1, 2, 3, 4}, RangeInt(0, 5, 1))
	assert.Equal(t, []int{5, 8}, RangeInt(5, 10, 3))
}

type foo struct {
	a int
	b int
}

// This is done to please structcheck
func (f foo) getAB() (int, int) {
	return f.a, f.b
}

// This is done to please structcheck
func TestFooStruct(t *testing.T) {
	a, b := foo{1, 2}.getAB()
	assert.Equal(t, 1, a)
	assert.Equal(t, 2, b)
}

func Test_ElementsMatch(t *testing.T) {
	var aNil []foo = nil
	var bNil []foo = nil

	assert.True(t, ElementsMatch(
		[]foo{{1, 2}, {3, 4}, {5, 6}},
		[]foo{{1, 2}, {3, 4}, {5, 6}}))

	assert.True(t, ElementsMatch(
		[]foo{{1, 2}, {3, 4}, {5, 6}},
		[]foo{{5, 6}, {1, 2}, {3, 4}}))

	assert.True(t, ElementsMatch(
		aNil,
		bNil))

	assert.False(t, ElementsMatch(
		[]foo{{1, 2}, {3, 4}, {5, 6}},
		[]foo{{5, 6}, {1, 2}}))

	assert.False(t, ElementsMatch(
		[]foo{{1, 2}, {1, 2}},
		[]foo{{5, 6}, {1, 2}}))

	assert.False(t, ElementsMatch(
		[]foo{{5, 6}, {1, 2}},
		[]foo{{1, 2}, {1, 2}}))

	assert.False(t, ElementsMatch(
		aNil,
		[]foo{{1, 2}, {1, 2}}))

	assert.False(t, ElementsMatch(
		[]foo{{5, 6}, {1, 2}},
		bNil))
}

func Test_mergeMap(t *testing.T) {
	type args struct {
		destination map[string]string
		sources     []map[string]string
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "Same Map",
			args: args{
				destination: map[string]string{
					"foo": "bar",
				},
				sources: []map[string]string{
					{
						"foo": "bar",
					},
				},
			},
			want: map[string]string{
				"foo": "bar",
			},
		}, {
			name: "Source missing key",
			args: args{
				destination: map[string]string{
					"foo": "bar",
				},
				sources: []map[string]string{
					{
						"foo": "bar",
						"key": "value",
					},
				},
			},
			want: map[string]string{
				"foo": "bar",
				"key": "value",
			},
		}, {
			name: "Destination missing key",
			args: args{
				destination: map[string]string{
					"foo": "bar",
					"key": "value",
				},
				sources: []map[string]string{
					{
						"foo": "bar",
					},
				},
			},
			want: map[string]string{
				"foo": "bar",
				"key": "value",
			},
		}, {
			name: "Empty Source",
			args: args{
				destination: map[string]string{
					"foo": "bar",
					"key": "value",
				},
				sources: []map[string]string{},
			},
			want: map[string]string{
				"foo": "bar",
				"key": "value",
			},
		}, {
			name: "Empty Destination",
			args: args{
				destination: map[string]string{},
				sources: []map[string]string{
					{
						"foo": "bar",
					},
				},
			},
			want: map[string]string{
				"foo": "bar",
			},
		}, {
			name: "Differing values for key",
			args: args{
				destination: map[string]string{
					"foo": "bar",
					"key": "value",
				},
				sources: []map[string]string{
					{
						"foo": "bar",
						"key": "value2",
					},
				},
			},
			want: map[string]string{
				"foo": "bar",
				"key": "value2",
			},
		}, {
			name: "Multiple source maps",
			args: args{
				destination: map[string]string{
					"foo": "bar",
					"baz": "foobar",
				},
				sources: []map[string]string{
					{
						"foo":   "qux",
						"waldo": "fred",
					},
					{
						"foo":  "quux",
						"quuz": "flob",
					},
				},
			},
			want: map[string]string{
				"foo":   "quux",
				"baz":   "foobar",
				"waldo": "fred",
				"quuz":  "flob",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			MergeMap(tt.args.destination, tt.args.sources...)

			eq := reflect.DeepEqual(tt.args.destination, tt.want)
			if !eq {
				t.Errorf("mergeMap() = %v, want %v", tt.args.destination, tt.want)
			}
		})
	}
}

func TestSearchMap(t *testing.T) {
	type args struct {
		mapToSearch map[string]any
		key         string
	}
	tests := []struct {
		name string
		args args
		want map[string]any
	}{
		{
			name: "Happy Path",
			args: args{
				mapToSearch: map[string]any{
					"key": map[string]any{
						"foo": "bar",
					},
				},
				key: "key",
			},
			want: map[string]any{
				"foo": "bar",
			},
		}, {
			name: "Deeply nested",
			args: args{
				mapToSearch: map[string]any{
					"foo": "bar",
					"a": map[string]any{
						"alpha": map[string]any{
							"foo": "bar",
						},
						"alpha1": map[string]any{
							"foo1": "bar1",
						},
					},
					"b": map[string]any{
						"bravo": "bar",
						"bravo1": map[string]any{
							"bravo111": map[string]any{
								"key": map[string]any{
									"foo": "bar",
								},
							},
						},
					},
					"c": map[string]any{
						"charlie": map[string]any{
							"foo": "bar",
						},
						"charlie1": map[string]any{
							"foo1": "bar1",
						},
					},
				},
				key: "key",
			},
			want: map[string]any{
				"foo": "bar",
			},
		}, {
			name: "Key Not Found",
			args: args{
				mapToSearch: map[string]any{
					"foo": "bar",
					"a": map[string]any{
						"alpha": map[string]any{
							"foo": "bar",
						},
						"alpha1": map[string]any{
							"foo1": "bar1",
						},
					},
					"b": map[string]any{
						"bravo": "bar",
						"bravo1": map[string]any{
							"bravo111": map[string]any{
								"wrong-key": map[string]any{
									"foo": "bar",
								},
							},
						},
					},
					"c": map[string]any{
						"charlie": map[string]any{
							"foo": "bar",
						},
						"charlie1": map[string]any{
							"foo1": "bar1",
						},
					},
				},
				key: "key",
			},
			want: map[string]any{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SearchMap(tt.args.mapToSearch, tt.args.key)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SearchMap() got = %v, want %v", got, tt.want)
			}
		})
	}
}
