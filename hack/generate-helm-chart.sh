#!/usr/bin/env bash
set -euo pipefail

kubebuilder="${KUBEBUILDER:-kubebuilder}"
expected_version="v4.16.0"
generated_workflow=".github/workflows/test-chart.yml"

if ! command -v "$kubebuilder" >/dev/null 2>&1; then
  echo "Kubebuilder ${expected_version} is required to regenerate the Helm chart." >&2
  exit 1
fi

actual_version="$("$kubebuilder" version | sed -n 's/^KubeBuilder:[[:space:]]*//p')"
if [[ "$actual_version" != "$expected_version" ]]; then
  echo "Expected Kubebuilder ${expected_version}, found ${actual_version:-unknown}." >&2
  exit 1
fi

temporary_directory="$(mktemp -d)"
cp Makefile "$temporary_directory/Makefile"
if [[ -f "$generated_workflow" ]]; then
  cp "$generated_workflow" "$temporary_directory/test-chart.yml"
  had_generated_workflow=true
else
  had_generated_workflow=false
fi

restore_project_files() {
  cp "$temporary_directory/Makefile" Makefile
  if [[ "$had_generated_workflow" == true ]]; then
    cp "$temporary_directory/test-chart.yml" "$generated_workflow"
  else
    rm -f "$generated_workflow"
  fi
  rm -f "$temporary_directory/Makefile" "$temporary_directory/test-chart.yml"
  rmdir "$temporary_directory"
}
trap restore_project_files EXIT

"$kubebuilder" edit --plugins=helm/v2-alpha
