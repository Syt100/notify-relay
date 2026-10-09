#!/bin/sh
# Inspect the actual ELF binary, independently of the image's platform label.
set -eu

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 IMAGE_REFERENCE [PLATFORM...]" >&2
  exit 2
fi
image=$1
shift
if [ "$#" -eq 0 ]; then set -- linux/amd64 linux/arm64 linux/arm/v7; fi
directory=$(mktemp -d)
container=
cleanup() {
  if [ -n "$container" ]; then docker rm -f "$container" >/dev/null 2>&1 || true; fi
  rm -rf "$directory"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

for platform do
  case "$platform" in
    linux/amd64) expected_machine=62; expected_class=2 ;;
    linux/arm64) expected_machine=183; expected_class=2 ;;
    linux/arm/v7) expected_machine=40; expected_class=1 ;;
    *) echo "Unsupported platform: $platform" >&2; exit 2 ;;
  esac
  if [ "${VERIFY_IMAGE_PULL:-true}" != false ]; then
    docker pull --platform "$platform" "$image"
  fi
  container=$(docker create --platform "$platform" "$image")
  docker cp "$container:/bridge" "$directory/bridge"
  docker rm "$container" >/dev/null
  container=
  magic=$(od -An -tx1 -N4 "$directory/bridge" | tr -d ' \n')
  class=$(od -An -tu1 -j4 -N1 "$directory/bridge" | tr -d ' \n')
  machine=$(od -An -tu2 -j18 -N2 "$directory/bridge" | tr -d ' \n')
  if [ "$magic" != 7f454c46 ] || [ "$class" != "$expected_class" ] || [ "$machine" != "$expected_machine" ]; then
    echo "Incorrect binary for $platform: ELF class=$class machine=$machine" >&2
    exit 1
  fi
  echo "Verified $platform: ELF class=$class machine=$machine"
  if [ "${VERIFY_IMAGE_PULL:-true}" != false ]; then
    # Classic Docker storage maps an index digest to only one platform.
    # Release this verification image before pulling the next platform.
    docker image rm "$image" >/dev/null
  fi
done
