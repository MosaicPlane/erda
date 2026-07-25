#!/usr/bin/env bash

community_image_output_args=()
community_image_output_should_load=false
community_image_output_temporary_path=

community_cleanup_image_output() {
  if [[ -n "${community_image_output_temporary_path}" ]]; then
    rm -f "${community_image_output_temporary_path}"
  fi
  community_image_output_args=()
  community_image_output_should_load=false
  community_image_output_temporary_path=
}

community_configure_image_output() {
  local output_type="${1:?output type is required}"
  local output_path="${2:-}"
  local image_name="${3:?image name is required}"

  community_cleanup_image_output
  case "${output_type}" in
    docker|oci|tar|registry)
      ;;
    *)
      echo "COMMUNITY_IMAGE_OUTPUT_TYPE must be docker, oci, tar, or registry" >&2
      return 1
      ;;
  esac
  if [[ "${output_path}" == *','* || "${output_path}" == *$'\n'* ]]; then
    echo "COMMUNITY_IMAGE_OUTPUT_PATH cannot contain a comma or newline" >&2
    return 1
  fi

  case "${output_type}" in
    docker)
      if [[ -z "${output_path}" ]]; then
        output_path="$(mktemp "${TMPDIR:-/tmp}/erda-community-image.XXXXXX")"
        community_image_output_should_load=true
        community_image_output_temporary_path="${output_path}"
      fi
      community_image_output_args=(
        --output "type=docker,dest=${output_path},rewrite-timestamp=true"
        --tag "${image_name}"
      )
      ;;
    oci)
      if [[ -z "${output_path}" ]]; then
        echo "COMMUNITY_IMAGE_OUTPUT_PATH is required for OCI output" >&2
        return 1
      fi
      community_image_output_args=(
        --output "type=oci,dest=${output_path},rewrite-timestamp=true"
        --tag "${image_name}"
      )
      ;;
    tar)
      if [[ -z "${output_path}" ]]; then
        echo "COMMUNITY_IMAGE_OUTPUT_PATH is required for tar output" >&2
        return 1
      fi
      community_image_output_args=(
        --output "type=tar,dest=${output_path},rewrite-timestamp=true"
      )
      ;;
    registry)
      if [[ -n "${output_path}" ]]; then
        echo "COMMUNITY_IMAGE_OUTPUT_PATH is not supported for registry output" >&2
        return 1
      fi
      community_image_output_args=(
        --output "type=registry,rewrite-timestamp=true"
        --tag "${image_name}"
      )
      ;;
  esac
}

community_load_image_output() {
  if [[ "${community_image_output_should_load}" == true ]]; then
    docker load --input "${community_image_output_temporary_path}" >/dev/null
  fi
}
