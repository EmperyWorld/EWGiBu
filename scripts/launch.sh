#!/bin/sh
# Interactive launcher for iacpack: GUI (serve), validate, or version.
# Does not publish, does not read secret values, does not run apply.
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
BIN="$ROOT/bin/iacpack"
ADDR=${IACPACK_ADDR:-127.0.0.1:7432}
WS=${IACPACK_WORKSPACE:-$ROOT/testdata/ui/two-service}

ensure_bin() {
  if [ -x "$BIN" ]; then
    return 0
  fi
  echo "собираю iacpack..." >&2
  (cd "$ROOT" && GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" make build)
}

policy_args() {
  workspace=$1
  if [ -f "$workspace/compile-policy.json" ]; then
    printf '%s\n' "--policy" "$workspace/compile-policy.json"
  fi
}

prompt() {
  # $1 = prompt text, $2 = default (optional)
  prompt_text=$1
  default=${2:-}
  if [ -n "$default" ]; then
    printf '%s [%s]: ' "$prompt_text" "$default" >&2
  else
    printf '%s: ' "$prompt_text" >&2
  fi
  IFS= read -r reply || reply=
  if [ -z "$reply" ] && [ -n "$default" ]; then
    reply=$default
  fi
  printf '%s\n' "$reply"
}

pick_workspace() {
  echo "" >&2
  echo "Workspace (каталог проекта с iacpack.yaml или пустой корень для UI):" >&2
  echo "  1) $WS  (текущий / IACPACK_WORKSPACE)" >&2
  echo "  2) $ROOT/testdata/ui/two-service" >&2
  echo "  3) $ROOT/examples/minimal" >&2
  echo "  4) $ROOT/examples/two-service" >&2
  echo "  5) текущий каталог ($(pwd))" >&2
  echo "  6) ввести путь" >&2
  choice=$(prompt "выбор" "1")
  case "$choice" in
    1) printf '%s\n' "$WS" ;;
    2) printf '%s\n' "$ROOT/testdata/ui/two-service" ;;
    3) printf '%s\n' "$ROOT/examples/minimal" ;;
    4) printf '%s\n' "$ROOT/examples/two-service" ;;
    5) printf '%s\n' "$(pwd)" ;;
    6)
      path=$(prompt "путь к workspace")
      if [ -z "$path" ]; then
        echo "пустой путь" >&2
        return 1
      fi
      printf '%s\n' "$path"
      ;;
    *)
      echo "неизвестный пункт: $choice" >&2
      return 1
      ;;
  esac
}

run_serve() {
  workspace=$(pick_workspace) || return 1
  if [ ! -d "$workspace" ]; then
    echo "каталога нет: $workspace" >&2
    return 1
  fi
  addr=$(prompt "адрес UI (только loopback)" "$ADDR")
  ensure_bin
  echo "" >&2
  echo "GUI: http://$addr/  workspace: $workspace" >&2
  echo "Остановка: Ctrl+C" >&2
  echo "" >&2
  set -- "$BIN" serve --workspace "$workspace" --addr "$addr"
  # Append policy flags without word-splitting on spaces in paths.
  while IFS= read -r arg; do
    [ -n "$arg" ] || continue
    set -- "$@" "$arg"
  done <<EOF
$(policy_args "$workspace")
EOF
  exec "$@"
}

run_validate() {
  workspace=$(pick_workspace) || return 1
  manifest="$workspace/iacpack.yaml"
  if [ ! -f "$manifest" ]; then
    echo "нет манифеста: $manifest" >&2
    return 1
  fi
  ensure_bin
  set -- "$BIN" validate --file "$manifest"
  while IFS= read -r arg; do
    [ -n "$arg" ] || continue
    set -- "$@" "$arg"
  done <<EOF
$(policy_args "$workspace")
EOF
  exec "$@"
}

run_version() {
  ensure_bin
  exec "$BIN" version
}

print_menu() {
  echo ""
  echo "iacpack — запуск"
  echo "  1) GUI — авторский интерфейс (serve)"
  echo "  2) Проверить манифест (validate)"
  echo "  3) Версия (version)"
  echo "  0) Выход"
  echo ""
  echo "Apply / plan / destroy — только из CLI (UI их не запускает)."
  echo "Секреты в аргументы не передаются."
}

main() {
  if [ ! -t 0 ]; then
    echo "нужен интерактивный терминал. Для неинтерактивного запуска:" >&2
    echo "  scripts/run-product.sh serve [workspace]" >&2
    echo "  scripts/run-product.sh validate [workspace]" >&2
    exit 2
  fi

  while true; do
    print_menu
    choice=$(prompt "выбор" "1")
    case "$choice" in
      1|gui|serve|ui)
        run_serve || true
        ;;
      2|validate)
        run_validate || true
        ;;
      3|version)
        run_version
        ;;
      0|q|quit|exit)
        exit 0
        ;;
      *)
        echo "неизвестный пункт: $choice" >&2
        ;;
    esac
  done
}

main "$@"
