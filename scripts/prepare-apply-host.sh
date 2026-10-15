#!/bin/sh
# Prepare a Linux host to run iacpack apply.
# Installs Podman and podman-compose, loads image tarballs when they are
# present, and checks the bundle, values, and secret files.
# Does not print secret bytes and does not start containers.
#
# Images are not inside the .iacbundle. apply runs `image inspect`, then
# `pull` when the engine does not have the ref. A registry tag can be pulled
# on this machine. A local tag (localhost/…, or a name with no registry)
# cannot. Those images stay available in two ways, and this script keeps both:
#   1. podman load of *.tar produced on the source machine
#   2. build them from the copied project repository, then apply
#
# Usage:
#   scripts/prepare-apply-host.sh [options] [kit-dir|bundle]
#     --iacpack PATH     iacpack binary (default: ./iacpack, else bin/iacpack)
#     --values PATH      values.json
#     --secrets DIR      directory of secret files, one file per secret id
#     --images DIR       directory of image tarballs to load (*.tar, *.tar.gz)
#     --skip-install     do not install packages
#     --engine podman|docker
set -eu

ENGINE=podman
SKIP_INSTALL=0
IACPACK=""
VALUES=""
SECRETS=""
IMAGES=""
TARGET=""

die() {
  echo "prepare-apply-host: $*" >&2
  exit 1
}

while [ $# -gt 0 ]; do
  case "$1" in
    --iacpack)
      IACPACK=${2:-}
      shift 2
      ;;
    --values)
      VALUES=${2:-}
      shift 2
      ;;
    --secrets)
      SECRETS=${2:-}
      shift 2
      ;;
    --images)
      IMAGES=${2:-}
      shift 2
      ;;
    --engine)
      ENGINE=${2:-}
      shift 2
      ;;
    --skip-install)
      SKIP_INSTALL=1
      shift
      ;;
    -h|--help)
      sed -n '2,20p' "$0"
      exit 0
      ;;
    --)
      shift
      break
      ;;
    -*)
      die "unknown option $1"
      ;;
    *)
      if [ -n "$TARGET" ]; then
        die "unexpected argument $1"
      fi
      TARGET=$1
      shift
      ;;
  esac
done

if [ -n "$TARGET" ] && [ -d "$TARGET" ]; then
  if [ -z "$VALUES" ] && [ -f "$TARGET/values.json" ]; then
    VALUES=$TARGET/values.json
  fi
  if [ -z "$IMAGES" ] && [ -d "$TARGET/images" ]; then
    IMAGES=$TARGET/images
  fi
  if [ -z "$SECRETS" ] && [ -d "$TARGET/secrets" ]; then
    SECRETS=$TARGET/secrets
  fi
fi

install_packages() {
  if [ "$SKIP_INSTALL" -eq 1 ]; then
    return 0
  fi
  if [ "$(id -u)" -ne 0 ]; then
    echo "нужны права root, чтобы поставить пакеты. Повторите через sudo или передайте --skip-install." >&2
    return 0
  fi
  if command -v dnf >/dev/null 2>&1; then
    dnf install -y podman podman-compose
  elif command -v apt-get >/dev/null 2>&1; then
    apt-get update
    apt-get install -y podman podman-compose
  else
    die "не найден dnf или apt-get. Поставьте podman и podman-compose вручную."
  fi
}

if [ "$ENGINE" = "podman" ] || [ "$ENGINE" = "docker" ]; then
  :
else
  die "--engine должен быть podman или docker"
fi

install_packages

if ! command -v "$ENGINE" >/dev/null 2>&1; then
  die "команда $ENGINE не найдена"
fi

if [ "$ENGINE" = "podman" ]; then
  if ! "$ENGINE" compose version >/dev/null 2>&1; then
    die "podman compose не отвечает. Пакет podman-compose нужен: apply вызывает именно «podman compose», не отдельный docker compose."
  fi
  sock=${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/podman/podman.sock
  if [ ! -S "$sock" ]; then
    echo "сокет $sock не найден. Для rootless:" >&2
    echo "  systemctl --user enable --now podman.socket" >&2
    echo "  loginctl enable-linger $(id -un)" >&2
  else
    echo "podman socket: $sock"
  fi
else
  if ! "$ENGINE" compose version >/dev/null 2>&1; then
    die "docker compose не отвечает. Нужен Compose plugin 2.24 или новее."
  fi
fi

if [ -z "$IACPACK" ]; then
  if [ -x ./iacpack ]; then
    IACPACK=./iacpack
  elif [ -x ./bin/iacpack ]; then
    IACPACK=./bin/iacpack
  fi
fi
if [ -n "$IACPACK" ]; then
  if [ ! -x "$IACPACK" ]; then
    die "iacpack не исполняемый: $IACPACK"
  fi
  echo "iacpack: $IACPACK"
  "$IACPACK" version || true
else
  echo "бинарник iacpack не найден рядом. Скопируйте bin/iacpack с машины, где собирали проект, или передайте --iacpack." >&2
fi

loaded=0
if [ -n "$IMAGES" ]; then
  if [ ! -d "$IMAGES" ]; then
    die "каталог образов не найден: $IMAGES"
  fi
  found=0
  for tar in "$IMAGES"/*.tar "$IMAGES"/*.tar.gz; do
    if [ ! -f "$tar" ]; then
      continue
    fi
    found=1
    echo "load $tar"
    "$ENGINE" load -i "$tar"
    loaded=$((loaded + 1))
  done
  if [ "$found" -eq 0 ]; then
    echo "в $IMAGES нет *.tar. Это нормально, если образы будут скачаны apply или собраны из репозитория." >&2
  else
    echo "загружено архивов образов: $loaded"
  fi
fi

if [ -n "$TARGET" ] && [ -f "$TARGET/images.txt" ]; then
  echo "проверка образов из images.txt"
  missing=0
  while IFS= read -r line || [ -n "$line" ]; do
    line=$(printf '%s' "$line" | tr -d '\r')
    [ -z "$line" ] && continue
    ref=$(printf '%s' "$line" | cut -f1)
    class=$(printf '%s' "$line" | cut -f2)
    case "$class" in
      pull)
        if ! "$ENGINE" image inspect "$ref" >/dev/null 2>&1; then
          echo "  pull при apply: $ref"
        fi
        ;;
      local-only|load)
        if ! "$ENGINE" image inspect "$ref" >/dev/null 2>&1; then
          echo "  нет локально, pull не поможет: $ref" >&2
          missing=$((missing + 1))
        fi
        ;;
    esac
  done < "$TARGET/images.txt"
  if [ "$missing" -gt 0 ]; then
    echo "локальные образы: загрузите их tar через --images или соберите из скопированного репозитория теми же тегами." >&2
  fi
fi

if [ -n "$SECRETS" ]; then
  if [ ! -d "$SECRETS" ]; then
    die "каталог секретов не найден: $SECRETS"
  fi
  n=0
  for f in "$SECRETS"/*; do
    if [ -f "$f" ]; then
      n=$((n + 1))
    fi
  done
  echo "файлов секретов: $n (содержимое не читалось)"
  if [ -f "${TARGET:-}/secrets.txt" ]; then
    while IFS= read -r id || [ -n "$id" ]; do
      id=$(printf '%s' "$id" | tr -d '\r')
      [ -z "$id" ] && continue
      if [ ! -f "$SECRETS/$id" ]; then
        echo "  нет файла секрета: $id" >&2
      fi
    done < "$TARGET/secrets.txt"
  fi
fi

if [ -n "$VALUES" ]; then
  if [ ! -f "$VALUES" ]; then
    die "values не найден: $VALUES"
  fi
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$VALUES" <<'PY'
import json, os, sys
path = sys.argv[1]
doc = json.load(open(path))
key = "stack2-llama-models-path"
val = doc.get(key) if isinstance(doc, dict) else None
if isinstance(val, str) and val:
    print(f"{key}={val}")
    if not os.path.isdir(val):
        print(f"каталог моделей не найден: {val}", file=sys.stderr)
        print("на этой машине запишите в values.json абсолютный путь к каталогу моделей", file=sys.stderr)
else:
    print(f"{key} не задан в {path}", file=sys.stderr)
PY
  else
    echo "python3 нет, путь моделей в values.json не проверен" >&2
  fi
fi

if [ ! -e /dev/nvidia0 ]; then
  echo "нет /dev/nvidia0. Сервис llama-chat в режиме devices-only без этих узлов не запустится. CDI для devices-only не нужен." >&2
else
  echo "nvidia: /dev/nvidia0 есть"
fi

echo
echo "Дальше apply. Каталог секретов передаётся целиком, значения в команду не входят."
echo "До контейнеров apply напишет, что будет скачано, каких локальных образов нет, какой путь или секрет или параметр отсутствует, и остановится с кодом 6."
if [ -n "$TARGET" ] && [ -n "$SECRETS" ] && [ -n "$IACPACK" ]; then
  values_arg=""
  if [ -n "$VALUES" ]; then
    values_arg=" --values \"$VALUES\""
  fi
  echo "  \"$IACPACK\" apply \"$TARGET\"$values_arg --secrets-dir \"$SECRETS\" --instance default --container-engine $ENGINE --yes"
fi
