#!/usr/bin/env bash
# Runs the given integration test suites (directories under tests/) one after another on the
# current kind cluster. A failing suite does not stop the following ones; the script exits with
# an error if any suite failed. Before each suite the cluster must be free of the previous suite's
# CassandraDatacenters, StatefulSets, PVCs and terminating namespaces, otherwise the remaining
# suites are not run.
#
# Logs of failed suites are kept in build/kubectl_dump/<suite>/, logs of passed suites are removed.
#
# Usage: IMG=<operator image> hack/run-integ-suites.sh <suite> [<suite> ...]

set -u

LEFTOVER_TIMEOUT=${LEFTOVER_TIMEOUT:-300}
DUMP_DIR=build/kubectl_dump

if [ $# -eq 0 ]; then
  echo "Usage: $0 <suite> [<suite> ...]" >&2
  exit 2
fi

leftovers() {
  local resources=statefulsets,persistentvolumeclaims
  # The CRD is removed with the operator, which also removes any CassandraDatacenter
  if kubectl get crd cassandradatacenters.cassandra.datastax.com > /dev/null 2>&1; then
    resources=cassandradatacenters,${resources}
  fi
  # "No resources found" goes to stderr
  kubectl get "${resources}" --all-namespaces --output name 2>/dev/null || echo "kubectl get ${resources} failed"
  kubectl get namespaces --no-headers 2>/dev/null | awk '$2 == "Terminating" { print "namespace/" $1 " Terminating" }'
}

wait_for_clean_cluster() {
  local deadline=$(( $(date +%s) + LEFTOVER_TIMEOUT ))
  local found
  while true; do
    found=$(leftovers)
    if [ -z "$found" ]; then
      return 0
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      echo "::error::Objects of a previous suite are still present after ${LEFTOVER_TIMEOUT}s:"
      echo "$found"
      return 1
    fi
    sleep 5
  done
}

summary_line() {
  echo "| $1 | $2 | $3 |" >> "${summary_file}"
}

summary_file=$(mktemp)
mkdir -p "${DUMP_DIR}"
failed=0
skip_rest=""

for suite in "$@"; do
  if [ -n "${skip_rest}" ]; then
    summary_line "${suite}" "not run" "${skip_rest}"
    failed=1
    continue
  fi

  echo "::group::Check for leftovers before ${suite}"
  if ! wait_for_clean_cluster; then
    echo "::endgroup::"
    summary_line "${suite}" "not run" "leftovers from the previous suite"
    skip_rest="leftovers from a previous suite"
    failed=1
    continue
  fi
  echo "::endgroup::"

  before=$(ls "${DUMP_DIR}")
  start=$(date +%s)
  echo "::group::Suite ${suite}"
  if M_INTEG_DIR="${suite}" make integ-test; then
    result="passed"
  else
    result="failed"
    failed=1
    echo "::error::Suite ${suite} failed"
  fi
  echo "::endgroup::"
  minutes=$(awk -v s=$(( $(date +%s) - start )) 'BEGIN { printf "%.1f", s / 60 }')
  summary_line "${suite}" "${result}" "${minutes} min"

  # Move the dumps of this suite to a directory named after it
  for entry in $(ls "${DUMP_DIR}"); do
    if ! grep -qxF "${entry}" <<< "${before}"; then
      if [ "${result}" = "passed" ]; then
        rm -rf "${DUMP_DIR:?}/${entry}"
      else
        mkdir -p "${DUMP_DIR}/${suite}"
        mv "${DUMP_DIR}/${entry}" "${DUMP_DIR}/${suite}/"
      fi
    fi
  done
done

{
  echo "### Integration test suites"
  echo
  echo "| Suite | Result | |"
  echo "|---|---|---|"
  cat "${summary_file}"
} | tee -a "${GITHUB_STEP_SUMMARY:-/dev/null}"

exit ${failed}
