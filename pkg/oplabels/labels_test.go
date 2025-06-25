package oplabels

import (
	"regexp"
	"strings"
	"testing"

	. "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/stretchr/testify/require"
)

/*
*
A valid label must be an empty string or consist of alphanumeric characters,
'-', '_' or '.', and must start and end with an alphanumeric.
*/
func TestLabelValueClean(t *testing.T) {
	var cleaned = CleanLabelValue("TestCluster")
	require.EqualValues(t, "TestCluster", cleaned,
		"expect label name to not be cleaned as no need")

	cleaned = CleanLabelValue("Test Cluster")
	require.EqualValues(t, "TestCluster", cleaned,
		"expect space to be cleaned")

	cleaned = CleanLabelValue("+!*(-_)cor @#$%^&rect_ LABEL.name-1=<>_?,.")
	require.EqualValues(t, "correct_LABEL.name-1", cleaned,
		"expect label name w/ outlier chars to be cleaned")

	cleaned = CleanLabelValue("cor!@#$%^&*()rect__ LABEL.name")
	require.EqualValues(t, "correct__LABEL.name", cleaned,
		"expect label name w/ inside chars to be cleaned")

	cleaned = CleanLabelValue("correct")
	require.EqualValues(t, "correct", cleaned,
		"expect label name without chars to be correct")

	cleaned = CleanLabelValue("")
	require.EqualValues(t, "", cleaned,
		"expect label name as empty without chars to be correct")

	cleaned = CleanLabelValue("-_!@#$%-^&*.()<>?._-&*.")
	require.EqualValues(t, "", cleaned,
		"expect label name as empty as contains all bad chars")

	cleaned = CleanLabelValue("-_!@#$%-^&*.()<>?._-&*.X")
	require.EqualValues(t, "X", cleaned,
		"expect label name as last char only")

	cleaned = CleanLabelValue("Y-_!@#$%-^&*.()<>?._-&*.")
	require.EqualValues(t, "Y", cleaned,
		"expect label name as first char only")
}

func TestWhitelistRegex(t *testing.T) {
	var whitelistRegex = regexp.MustCompile(`(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?`)
	var unclean = "+!*(-_)cor @#$%^&rect _ LABEL.name-1=<>_?,."
	var regexpResult = whitelistRegex.FindAllString(strings.ReplaceAll(unclean, " ", ""), -1)
	require.EqualValues(t, "correct_LABEL.name-1", strings.Join(regexpResult, ""))

	unclean = "+!*(-_)cor @#$   %^&re _c-t-.LABEL.name-1=<>_?,."
	regexpResult = whitelistRegex.FindAllString(strings.ReplaceAll(unclean, " ", ""), -1)
	require.EqualValues(t, "corre_c-t-.LABEL.name-1", strings.Join(regexpResult, ""))
}
