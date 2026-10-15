#!/usr/bin/env bash
set -euo pipefail
tag=${1:?usage: package-release.sh vMAJOR.MINOR.PATCH}
[[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Invalid release tag' >&2; exit 1; }
version=${tag#v}
[[ $version == "$(sed -n 's/^VERSION ?= //p' Makefile)" ]] || { echo 'Version mismatch' >&2; exit 1; }
out=dist/release
stage=$(mktemp -d)
license_container=
trap 'if [[ -n $license_container ]]; then docker rm "$license_container" >/dev/null; fi; rm -rf "$stage"' EXIT
mkdir -p "$out"
docker build --platform linux/amd64 -t "luna-ebpf:$version" .
license_container=$(docker create "luna-ebpf:$version")
docker cp "$license_container:/licenses" "$stage/licenses"
for arch in amd64 arm64; do
  dir="$stage/luna-ebpf_${version}_linux_${arch}"
  mkdir -p "$dir"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$dir/luna-ebpf" ./cmd/luna-ebpf
  cp -r "$stage/licenses" "$dir/licenses"
  cp LICENSE THIRD_PARTY_NOTICES.md configs/config.example.yaml "$dir/"
  tar -czf "$out/luna-ebpf_${version}_linux_${arch}.tar.gz" -C "$stage" "$(basename "$dir")"
done
docker save "luna-ebpf:$version" | gzip > "$out/luna-ebpf_${version}_docker_amd64.tar.gz"
python3 - "$version" "$out" <<'PY'
import json,pathlib,subprocess,sys,tarfile
version,out=sys.argv[1],pathlib.Path(sys.argv[2])
config=json.loads(subprocess.check_output(['docker','compose','-f','docker-compose.yaml','config','--format','json']))
config.pop('name',None)
service=config['services']['luna-ebpf']
service.pop('build',None)
service['pull_policy']='never'
assert service['image']=='luna-ebpf:'+version
# JSON is valid YAML and accepted directly by Docker Compose.
(out/'docker-compose.yaml').write_text(json.dumps(config,indent=2)+'\n')
with tarfile.open(out/f'luna-ebpf_{version}_docker_amd64.tar.gz') as archive:
    manifest=json.load(archive.extractfile('manifest.json'))
    assert [tag for item in manifest for tag in item.get('RepoTags',[])]==['luna-ebpf:'+version]
PY
cp deploy/grafana/luna-ebpf-dashboard.json "$out/"
cp deploy/prometheus/luna-alerts.yaml "$out/"
(cd "$out" && sha256sum ./*.tar.gz docker-compose.yaml luna-ebpf-dashboard.json luna-alerts.yaml > SHA256SUMS)
