# Работа из консоли

Полный разбор импорта, второй базы, секретов, GPU и переносимого комплекта:
[GUIDE.md](GUIDE.md).

Команды ниже совпадают с `iacpack <command> -h`. Секретные значения в аргументы не передаются: каталог `--secrets-dir` или путь к файлу `--secret-file id=/path`.

Бинарник собирается из корня репозитория:

```bash
cd /path/to/EWGiBu
make build
./bin/iacpack version
```

Дальше в примерах `iacpack` — это `./bin/iacpack` или тот же бинарник в `PATH`.

## Авторский UI

Интерактивное меню (GUI / validate / version):

```bash
scripts/launch.sh
```

Пункт **1** запускает авторский UI. Скрипт предлагает workspace и адрес
(по умолчанию `127.0.0.1:7432`). Без TTY скрипт завершается с подсказкой
перейти на `run-product.sh`.

Без меню, сразу UI:

```bash
scripts/run-product.sh
```

Открывает http://127.0.0.1:7432/ и workspace `testdata/ui/two-service`. Если рядом с манифестом лежит `compile-policy.json`, скрипт передаёт его в `--policy`.

Свой каталог и адрес:

```bash
scripts/run-product.sh serve /path/to/project
IACPACK_ADDR=127.0.0.1:7432 scripts/run-product.sh serve /path/to/project
```

То же без скрипта:

```bash
iacpack serve \
  --workspace /path/to/project \
  --policy /path/to/project/compile-policy.json \
  --addr 127.0.0.1:7432
```

`--policy` нужен, если в проекте есть внешние сети (`externalNetworks: true`). Без файла используется встроенная политика: внешние сети запрещены.

Остановка: `Ctrl+C` в этом терминале.

Проверка манифеста тем же скриптом:

```bash
scripts/run-product.sh validate
scripts/run-product.sh validate /path/to/project
```

Каждый проект — отдельный каталог с собственным `iacpack.yaml`. В UI кнопка «Начать новый проект» создаёт `projects/<id>/` внутри корня, с которым запущен `serve`. Из консоли тот же приём:

```bash
mkdir -p /path/to/studio/projects/shop
iacpack init --output /path/to/studio/projects/shop/iacpack.yaml
iacpack serve --workspace /path/to/studio/projects/shop --addr 127.0.0.1:7432
```

Переключение в UI меняет только каталог внутри этого корня. Абсолютный путь снаружи корня API не принимает.

## Новый манифест

```bash
mkdir -p /path/to/project
iacpack init --output /path/to/project/iacpack.yaml
```

`init` не перезаписывает существующий файл.

## Импорт Compose

Пути Compose и Dockerfile — относительно `--workspace`. Propose ничего не пишет на диск.

```bash
iacpack import propose \
  --workspace /path/to/project \
  --compose compose.yaml \
  --compose stack2.yml \
  --profile companion-tool-router \
  --name shop \
  --output json
```

- Повтор `--compose` и `--profile` добавляет файлы и профили. Несколько имён в одном флаге можно перечислить через запятую.
- `--import-strict` поднимает policy-отказы до error.
- Сервис с `profiles:` попадает в черновик только если профиль передан. Иначе диагностика `IMPORT.PROFILE_SKIPPED`.
- Если в Compose нет `user`, propose читает локальный `Config.User`. Число записывается в `service.user`. Имя — диагностика `IMPORT.USER_NAME`. Нет образа — последняя инструкция `USER` в Dockerfile, иначе предупреждение `IMPORT.USER_ABSENT`. Повторный импорт не затирает `user` из черновика: `--existing-project-file` или повторный Propose в UI.
- Сервисы с одной pull-ссылкой получают `environmentFrom` и оставляют только отличающиеся ключи (`IMPORT.ENV_FROM`). `plan` тогда не печатает `PLAN.ENV_MISSING` для этой пары.
- Теги образов (`repo:tag`) и `image.ref: {parameter: id}` без default дают `mode: development`. `release` допустим, только если каждый ref уже `repository@sha256:<64-hex>`.
- Вложенный `${A:-${B}}` без литерала — параметр без default, не строка `${…}` в `image.ref`.
- `devices:` и `privileged` исключают сервис. Это warning, пока нет `--import-strict`.
- Абсолютный host bind не копируется в черновик. Это warning, сервис остаётся. `--import-strict` делает его error.
- Фрагмент другого файла с `!override` / `!reset` и без `image` заменяет поле сервиса с тем же id и не ломает `propose.ok`.

Сохранить черновик и создать пустые каталоги bind/build:

```bash
iacpack import write-draft \
  --workspace /path/to/project \
  --project-file /path/to/draft.json \
  --path imports/import-draft.json \
  --replace \
  --output json

iacpack import stub-preview \
  --workspace /path/to/project \
  --project-file imports/import-draft.json \
  --output json

iacpack import prepare \
  --workspace /path/to/project \
  --project-file imports/import-draft.json \
  --path imports/import-draft.json \
  --replace \
  --output json
```

`prepare` — это `write-draft`, затем `resource-stubs`. `stub-preview` только печатает план, каталоги не создаёт.

Поиск файлов Compose в дереве (не импорт):

```bash
iacpack inspect --discover --workspace /path/to/project --output json
```

## Проверка, lock, сборка

Все команды ниже запускать из каталога проекта или с явным `--file`.

```bash
cd /path/to/project
iacpack validate --file iacpack.yaml --policy compile-policy.json
iacpack lock --file iacpack.yaml --policy compile-policy.json
iacpack validate --file iacpack.yaml --policy compile-policy.json --locked
iacpack build --file iacpack.yaml --policy compile-policy.json \
  --format bundle --output .iacpack-out/bundle.iacbundle
iacpack verify .iacpack-out/bundle.iacbundle
```

Черновик `imports/import-draft.json` проверяется от корня проекта, не от каталога `imports/`:

```bash
iacpack validate --file imports/import-draft.json --policy compile-policy.json
```

`--locked` сравнивает манифест с `iacpack.lock.json`. После правки `iacpack.yaml`, политики или схемы нужен новый `lock`, иначе preview и build сообщают `LOCK.DRIFT`.

`release` требует digest у каждого образа. Тег в development даёт warning `LOCK.IMAGE_UNRESOLVED` и не блокирует preview. `--update-images` сам по реестру образы не резолвит: digest нужно записать в `ref` заранее (`repository@sha256:<64-hex>`).

Каталог вместо бандла:

```bash
iacpack build --file iacpack.yaml --format directory --output /tmp/iacpack-out
```

Цель `build` не должна уже существовать.

## План и apply

Секрет — файл на диске, не значение в командной строке.

```bash
iacpack plan .iacpack-out/bundle.iacbundle \
  --values values.json \
  --secrets-dir ./secrets

iacpack apply .iacpack-out/bundle.iacbundle \
  --values values.json \
  --secrets-dir ./secrets \
  --container-engine podman \
  --yes
```

`--secrets-dir` — каталог, где имя файла равно id секрета. Один id можно заменить флагом `--secret-file id=/path`. До записи состояния apply останавливается с кодом 6, если нет локального образа (`APPLY.IMAGE_LOCAL`), путь из values не существует (`APPLY.HOST_PATH`), нет файла секрета (`SECRET.MISSING`) или параметр deploy без значения (`PARAM.MISSING`). Теги из реестра, которых ещё нет, перечисляются в `APPLY.IMAGE_PULL`. Подсказка и сам apply вызывают `podman pull` с этими именами через пробел, без запятых. Затем контейнеры стартуют. Подготовка машины: `scripts/prepare-apply-host.sh`.

Движок по умолчанию: Podman, если доступен, иначе Docker. Явно: `--container-engine podman|docker` или `IACPACK_CONTAINER_ENGINE`. Сокет: `--docker-socket` или `IACPACK_DOCKER_SOCKET`.

Каждый сервис: `compose up -d --no-deps --force-recreate <service>`. Ожидание
healthy — через `ps`/`inspect` по labels (не `compose ps --status`). Сбой
start/ready одного сервиса не обрывает остальные; в диагностике имя сервиса
(и CDI-устройство для GPU). Writable bind с `:z` → `read_only: false`; если
mount всё же RO — `APPLY.ACTION` с сервисом и путём. В command строки
`yes`/`no`/`on`/`off` квотируются; пустой аргумент остаётся `""`. Задачи
lifecycle идут как `compose --profile iacpack-task run -T`. `-T` нужен, чтобы
`psql` не ждал пароль на TTY. У задачи есть `entrypoint` (заменяет ENTRYPOINT
образа; `[]` очищает его) и `user` (`0`, `uid:gid` или имя). Если `entrypoint`
задан, перед ним выполняется `cd` в рабочий каталог образа. `command` —
аргументы, не опции `kc.sh`. File-secret монтируется из `secrets/<id>` рядом
с rendered compose. `gpu: devices-only` монтирует `/dev/nvidia*` и на SELinux
enforcing ставит этому сервису `label=disable`. Повторный apply того же
архива печатает `noop`, пока не изменились хеш задачи, host path или штамп
GPU. Подробнее: [runtime-apply-v1.md](../reference/runtime-apply-v1.md) и
[GUIDE.md](GUIDE.md).

Если черновик в текущем каталоге (или `--workspace`) новее lock внутри bundle, plan и apply печатают `lock устарел` и `iacpack diff --from …` и не стартуют контейнеры (код 6).

```bash
iacpack explain .iacpack-out/bundle.iacbundle api
iacpack bundle \
  --workspace /path/to/project \
  --output-dir /path/to/kit \
  --policy compile-policy.json \
  --embed-local-images
```

`explain` показывает образ, `user`, gpu, `ldLibraryPath`, устройства, ключи окружения и `environmentFrom`. `bundle` пишет lock, `values.json` без секретов, `secrets.txt` (только имена), `images.txt`, пустой шаблон `host-paths.json` и `project.iacbundle`. Пустые пути и невстроенный локальный образ — предупреждения, код 0. `plan` и `apply` каталога останавливаются, пока пути не абсолютные и образы не загружены, затем стартуют этот архив. Подробно: [GUIDE.md](GUIDE.md).

`<project>` в status и destroy — это `metadata.name`, не путь к бандлу.

```bash
iacpack status shop --instance default
iacpack doctor --container-engine podman
iacpack destroy shop --container-engine podman --instance default --yes
```

Тома по умолчанию сохраняются. Удаление только с `--delete-volumes` и отдельным подтверждением.

Один lifecycle-task:

```bash
iacpack task run shop migrate --instance default --container-engine podman --yes
```

Вторая база на уже работающем Postgres — блок `spec.databases`, не второй
контейнер. Compile добавляет задачу `db-<id>` после `healthy` и подставляет
DSN в `urlParameter`. Пароль остаётся ссылкой на секрет. SQL с новым SHA-256
исполняется снова и должен выдержать повтор. Пример и поля: [GUIDE.md](GUIDE.md).

Импорт сам задачи миграции не создаёт. `baseline.sql` + `pending/` дают
`IMPORT.MIGRATE_GAP` и шаблон task. Каталог `applied/` не проигрывается.
Сервис API: `dependsOn` migrate с `completedSuccessfully`. Пример argv-runner:
`docs/release/examples/pack-migrate`.

Повтор прерванного apply:

```bash
iacpack resume shop --instance default
```

## Полезные флаги

| Флаг | Где |
| --- | --- |
| `--output text\|json` | почти все команды; JSON сверяется со `schemas/cli/v1` |
| `--policy <path>` | validate, lock, build, serve, inspect inputs |
| `--values <path>` / `--set k=v` | параметры без секретов |
| `--syntax-only` | validate только разбирает манифест |
| `--dry-run` | destroy, resume, release init-templates |

Справка по команде: `iacpack import -h`, `iacpack serve -h`, `iacpack apply -h`.
