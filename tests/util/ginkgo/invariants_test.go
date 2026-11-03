// Copyright DataStax, Inc.
// Please see the included license file for details.

package ginkgo_util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseNodetoolStatus(t *testing.T) {
	output := `Datacenter: dc1
===============
Status=Up/Down
|/ State=Normal/Leaving/Joining/Moving
--  Address     Load        Tokens  Owns (effective)  Host ID                               Rack
UN  10.244.1.5  123.45 KiB  16      100.0%            5d8b1a2e-1c2d-4e5f-8a9b-0c1d2e3f4a5b  r1
DN  10.244.2.6  ?           16      100.0%            6d8b1a2e-1c2d-4e5f-8a9b-0c1d2e3f4a5b  r2

Datacenter: My_Super_Dc
=======================
Status=Up/Down
|/ State=Normal/Leaving/Joining/Moving
--  Address           Load       Tokens  Owns (effective)  Host ID                               Rack
UJ  fd00:10:244::7    97.1 KiB   16      ?                 7d8b1a2e-1c2d-4e5f-8a9b-0c1d2e3f4a5b  rack-1
`
	nodes := parseNodetoolStatus(output)
	require.Len(t, nodes, 2)
	require.Equal(t, []NodetoolNodeInfo{
		{Status: "up", State: "normal", Address: "10.244.1.5", HostId: "5d8b1a2e-1c2d-4e5f-8a9b-0c1d2e3f4a5b", Rack: "r1"},
		{Status: "down", State: "normal", Address: "10.244.2.6", HostId: "6d8b1a2e-1c2d-4e5f-8a9b-0c1d2e3f4a5b", Rack: "r2"},
	}, nodes["dc1"])
	require.Equal(t, []NodetoolNodeInfo{
		{Status: "up", State: "joining", Address: "fd00:10:244::7", HostId: "7d8b1a2e-1c2d-4e5f-8a9b-0c1d2e3f4a5b", Rack: "rack-1"},
	}, nodes["My_Super_Dc"])
}
