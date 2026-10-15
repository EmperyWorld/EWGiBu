# Техническое задание: IaC Packager (`iacpack`)

**Статус:** утверждённая базовая спецификация для реализации  
**Версия документа:** 1.0  
**Целевая версия продукта:** v1.x  
**Последнее обновление:** 2026-09-28  
**Лицензия продукта:** MIT
**Отказ от ответственности:** программное обеспечение предоставляется «как есть» (AS IS); авторы и контрибьюторы не несут ответственности за любой ущерб или последствия использования — см. `LICENSE` и `NOTICE`.  
**Дистрибуция исходников:** репозиторий содержит только исходный код и контракты; скомпилированные бинарники не хранятся и не публикуются этим рабочим процессом.  
 

## 0. Назначение документа

Этот документ определяет продуктовые, функциональные, архитектурные, эксплуатационные и качественные требования к **IaC Packager** — инструменту, который компилирует декларативное описание многосервисного приложения и явно объявленные файлы в переносимый, проверяемый и воспроизводимый deployment bundle.

Документ является нормативным. Термины **MUST / ДОЛЖЕН**, **MUST NOT / НЕ ДОЛЖЕН**, **SHOULD / СЛЕДУЕТ** и **MAY / МОЖЕТ** используются в смысле RFC 2119.

Если реализация требует изменить зафиксированный здесь контракт, сначала создаётся ADR и одновременно обновляются спецификация, JSON Schema, тестовые fixtures и план реализации. Нельзя молча менять поведение ради упрощения кода.

## 1. Краткое описание продукта

`iacpack` принимает:

1. `iacpack.yaml` или эквивалентный JSON-манифест;
2. `iacpack.lock.json`;
3. только явно объявленные локальные ресурсы;
4. явно переданные build/deploy parameters и ссылки на секреты.

Продукт выполняет pipeline:

```text
load → validate → resolve → compile → plan → render → verify → package
```

и создаёт:

- канонический deployment bundle (`*.iacbundle`);
- Docker Compose-конфигурацию;
- документацию и `.env.example` без секретов;
- lock/provenance/checksum metadata;
- при выбранном distribution profile — тонкий `install.sh` и runtime-бинарник `iacpack`.

Тот же runtime умеет проверить bundle, построить deployment plan, развернуть его через Docker Compose, показать состояние, продолжить прерванное развёртывание и безопасно удалить принадлежащие проекту ресурсы.

Визуальный редактор является клиентом того же Core Engine. Он не реализует отдельную версию валидации или генерации.

## 2. Цели и критерии успеха

### 2.1. Цели

- Единый декларативный source of truth для упаковки и развёртывания проекта.
- Воспроизводимые артефакты из одних и тех же объявленных входов.
- Переносимость без изменения исходного проекта.
- Безопасная работа с недоверенными манифестами, путями и архивами.
- Предсказуемый CLI для локальной разработки и CI/CD.
- Установка многосервисного приложения без ручного написания сложных shell-скриптов.
- Общий контракт для CLI, локального API и Web UI.
- Диагностика, пригодная одновременно для человека и автоматизации.

### 2.2. Измеримые показатели

- Два build-запуска с одинаковыми байтами входов, lockfile, версией `iacpack` и параметрами дают byte-identical bundle.
- `validate`, `inspect`, `build` и preview не запускают код проекта, контейнеры или hooks и не используют сеть.
- В bundle не попадают необъявленные файлы и значения объявленных секретов.
- Reference-проект из acceptance fixtures проходит build → verify → plan → apply → health → re-apply → destroy.
- Повторный `apply` не создаёт дубликаты управляемых ресурсов.
- Любая ошибка имеет стабильный code, location, hint и корректный exit code.

## 3. Термины

| Термин | Определение |
|---|---|
| Authoring manifest | Редактируемый пользователем `iacpack.yaml`/JSON. |
| Canonical manifest | Нормализованное JSON-представление authoring manifest. |
| Lockfile | `iacpack.lock.json` с точными digest входов, образов, schema и renderer. |
| IR | Неизменяемое внутреннее представление после валидации и resolution. |
| Deployment plan | Детерминированный DAG действий без секретных значений. |
| Bundle | Проверяемый deterministic archive с расширением `.iacbundle`. |
| Distribution | Bundle вместе с runtime-бинарником и тонким installer wrapper. |
| Workspace | Канонический корень, внутри которого разрешены authoring inputs. |
| Parameter | Несекретное типизированное значение build- или deploy-фазы. |
| Secret | Значение, предоставляемое оператором только на deploy-фазе. |
| Provider profile | Версионированный контракт target runtime, например `compose-docker/v1`. |
| Managed resource | Ресурс с ownership labels и записью в journal `iacpack`. |
| Task | Декларативная one-shot container-команда lifecycle; не host shell hook. |

## 4. Пользователи и основные сценарии

### 4.1. Роли

- **Application developer:** описывает сервисы, ресурсы, параметры и lifecycle tasks.
- **Release engineer:** фиксирует образы, создаёт и подписывает bundle.
- **Operator:** проверяет, планирует, применяет, обновляет и удаляет deployment.
- **Platform/security engineer:** задаёт policy и проверяет provenance.
- **Visual author:** редактирует тот же манифест через Web UI.

### 4.2. Ключевые сценарии

1. Создать минимальный манифест через `iacpack init`.
2. Провалидировать проект локально и в CI.
3. Зафиксировать локальные inputs и OCI image digests.
4. Собрать воспроизводимый online bundle.
5. Проверить checksum/signature/policy до любых изменений системы.
6. Просмотреть план и применить deployment.
7. Повторно применить тот же bundle без дублирования ресурсов.
8. Обновить проект новым bundle с сохранением persistent volumes.
9. Продолжить прерванный apply на основании journal.
10. Удалить управляемые ресурсы; данные удалить только отдельным явным флагом.
11. Импортировать, редактировать и экспортировать манифест через Web UI без расхождения с CLI.

## 5. Scope и поэтапная поставка

### 5.1. MVP

MVP включает:

- Go Core Engine и CLI;
- strict YAML/JSON manifest `iacpack.io/v1alpha1`;
- JSON Schema 2020-12;
- загрузку, diagnostics, path confinement и semantic validation;
- image-based Docker Compose services;
- parameters, networks, named volumes, mounts, healthchecks и dependencies;
- lockfile;
- deterministic directory output и `.iacbundle`;
- `init`, `validate`, `lock`, `build`, `verify`, `inspect`, `schema`, `version`;
- unit, golden, property, fuzz и reproducibility tests.

### 5.2. v1.0

Дополнительно к MVP:

- lifecycle tasks в контейнерах;
- `plan`, `doctor`, `apply`, `status`, `resume`, `destroy`;
- journaled Docker Compose provider;
- thin `install.sh` distribution для Linux amd64/arm64;
- online bundle и опциональный offline OCI image payload;
- signing, checksums, SBOM и release pipeline.

### 5.3. v1.1

- loopback-only local API;
- React Web UI и graph designer;
- explicit save, ETag и conflict handling;
- live validation/preview/file tree;
- semantic import/export;
- WCAG 2.2 AA.

Дополнительно (ADR-016…019 / фазы P16–P19): reviewable Compose/Dockerfile
import propose→Accept→Save; widened discover и multi-Compose; parameter/secret
stubs без значений; явные GitHub CI/release templates без авто-публикации.

**Статус поставки:** публичная линия **`1.1.0`** закрывает §5.1–5.3 (MVP → v1.0
runtime + v1.1 authoring; фазы P00–P19). Перед signed tag оператор обязан прогнать
`make docker-e2e` на хосте с Docker и подтвердить CI `reproduce-multi-os`.
Публикация — только по `docs/release/PUBLISH.md`. См. `TRACEABILITY.md`,
`KNOWN_LIMITATIONS.md`, `IMPLEMENTATION_PLAN.md` §8.

### 5.4. За пределами v1.x

Следующее запрещено добавлять в manifest schema без отдельного ADR, threat model и conformance suite:

- Podman provider;
- Kubernetes/k3s generator;
- Windows/PowerShell distribution;
- remote Docker contexts;
- arbitrary host hooks;
- third-party executable plugins/templates;
- hosted multi-tenant UI;
- автоматическая публикация в GitHub из пользовательского приложения;
- «вся инфраструктура внутри одного Docker image».

## 6. Поддерживаемая платформа

### 6.1. Tooling

- Core/CLI/runtime: Go 1.27.x, patch version фиксируется в `go.mod`/toolchain и CI.
- CLI framework: Cobra.
- YAML AST: `gopkg.in/yaml.v3` с дополнительными строгими проверками.
- JSON Schema: Draft 2020-12.
- UI: Node.js 24 LTS, pnpm 10, TypeScript strict, React 19, Vite, `@xyflow/react`.
- API: Go `net/http`, OpenAPI 3.1, generated client.

### 6.2. Target runtime v1

- Linux amd64 и arm64.
- Docker Engine 24+.
- Docker Compose plugin 2.24+, вызов только как `docker compose`.
- Linux containers.
- Локальный Docker daemon.
- Bash 4+ нужен только для thin installer wrapper.

CLI build/validate поддерживается на Linux, macOS и Windows. `apply` в v1 поддерживается только на Linux. Unsupported platform MUST завершаться явной диагностикой, а не best-effort поведением.

## 7. Архитектурные принципы и инварианты

1. **Manifest-first:** manifest и lockfile являются источниками истины.
2. **Pure compiler:** compile/build не имеют скрытых side effects.
3. **Explicit inputs:** build зависит только от объявленных файлов и параметров.
4. **Non-invasive:** исходный workspace остаётся read-only.
5. **No arbitrary rewriting:** Dockerfile и произвольные config-файлы не переписываются.
6. **Typed path domains:** workspace, bundle, container, managed host и external host paths не смешиваются.
7. **Secret references only:** секретные значения отсутствуют в manifest, lock, bundle, docs и state journal.
8. **Fail closed:** неизвестные поля/версии/возможности являются ошибкой.
9. **Determinism by construction:** сортировка, canonical serialization и normalized metadata обязательны.
10. **One engine:** CLI/API/UI используют одни application services и diagnostics.
11. **Plan before mutate:** runtime сначала verify/preflight/plan, затем применяет.
12. **Compensation, not fake rollback:** обещается только ограниченная очистка обратимых ресурсов текущего run.
13. **Persistent data safety:** volumes сохраняются по умолчанию.
14. **No ambient behavior:** cwd, locale, timezone, umask, `.env`, `COMPOSE_*` и host environment не должны неявно менять результат.

## 8. Высокоуровневая архитектура

```text
CLI ─────┐
HTTP API ├──> Application services ──> Domain / immutable IR
Web UI ──┘              │
                        ├── Manifest loader + schema validation
                        ├── Input resolver + lockfile
                        ├── Semantic validators + policy
                        ├── DAG planner
                        ├── Compose renderer
                        ├── Bundle writer/verifier
                        └── Deployment runtime
                                   │
                                   ├── Docker Compose provider
                                   └── Journal/state store
```

### 8.1. Dependency direction

- `adapters → application → domain`;
- domain не импортирует CLI, HTTP, filesystem, process или Docker adapters;
- clock, filesystem, process runner, registry и randomness передаются интерфейсами;
- CLI и HTTP handlers не вызывают друг друга subprocess-командами;
- runtime потребляет подписанный/проверенный compiled deployment document, а не заново интерпретирует authoring YAML.

### 8.2. Предлагаемая структура monorepo

```text
cmd/iacpack/
internal/
  app/
  diagnostic/
  manifest/
    loader/
    v1alpha1/
    migrate/
  domain/
    ir/
    plan/
  inputs/
  lock/
  policy/
  render/compose/
  bundle/
  runtime/
    engine/
    dockercompose/
    journal/
    secrets/
  adapters/
    cli/
    http/
    filesystem/
    registry/
api/openapi/iacpack-v1.yaml
schemas/manifest/v1alpha1/schema.json
schemas/policy/v1/schema.json
schemas/deployment/v1/schema.json
schemas/bundle/v1/schema.json
schemas/cli/v1/
web/
docs/adr/
docs/reference/
examples/
testdata/
  manifests/
  golden/
  malicious/
  bundles/
e2e/
```

## 9. Manifest contract

### 9.1. Версии

Используются независимые версии:

- `apiVersion` — версия authoring schema;
- `metadata.version` — SemVer-релиз упаковываемого проекта;
- `bundle.formatVersion` — версия bundle format;
- provider profile — например `compose-docker/v1`.

Начальная authoring version: `iacpack.io/v1alpha1`. Несовместимые изменения требуют нового `apiVersion`. Автоматическая тихая миграция запрещена; используется `iacpack manifest migrate --to ... --output ...`.

### 9.2. Формат

- YAML 1.2 subset является основным authoring format.
- JSON с теми же данными обязан иметь идентичную семантику.
- Поддерживается ровно один document.
- Duplicate keys, merge keys, aliases, anchors, custom tags и non-string map keys запрещены.
- Unknown fields запрещены, кроме явно описанных `x-*` extension points.
- Максимальный размер manifest по умолчанию: 2 MiB.
- Максимальная глубина: 100.
- Максимум: 500 services, 500 tasks, 10 000 declared files после expansion.
- Идентификаторы соответствуют `^[a-z][a-z0-9-]{0,62}$` и уникальны без учёта регистра.

### 9.3. Полный пример

```yaml
apiVersion: iacpack.io/v1alpha1
kind: Project

metadata:
  name: acme-shop
  version: 1.4.0
  description: Acme Shop deployment

spec:
  target:
    profile: compose-docker/v1
  mode: release

  parameters:
    http-port:
      phase: deploy
      type: integer
      default: 8080
      minimum: 1
      maximum: 65535
      description: Loopback HTTP port
    db-user:
      phase: deploy
      type: string
      default: acme

  secrets:
    db-password:
      required: true
      delivery: file
      description: Database password

  images:
    app:
      ref: ghcr.io/acme/shop@sha256:1111111111111111111111111111111111111111111111111111111111111111
      delivery: pull
    postgres:
      ref: docker.io/library/postgres@sha256:2222222222222222222222222222222222222222222222222222222222222222
      delivery: pull

  resources:
    app-config:
      source: config
      include:
        - app.yaml
      destination: config
      mode: "0644"

  networks:
    backend: {}

  volumes:
    db-data:
      retention: retain

  services:
    db:
      image: postgres
      environment:
        POSTGRES_DB:
          literal: acme
        POSTGRES_USER:
          parameter: db-user
        POSTGRES_PASSWORD_FILE:
          literal: /run/secrets/db-password
      secrets:
        - ref: db-password
          target: /run/secrets/db-password
      mounts:
        - volume: db-data
          target: /var/lib/postgresql/data
      networks: [backend]
      healthcheck:
        command: ["pg_isready", "-U", "acme"]
        interval: 5s
        timeout: 3s
        retries: 20

    app:
      image: app
      dependsOn:
        - target: db
          condition: healthy
      ports:
        - container: 8080
          host:
            parameter: http-port
          bind: 127.0.0.1
          protocol: tcp
      mounts:
        - resource: app-config
          source: app.yaml
          target: /etc/acme/app.yaml
          readOnly: true
      networks: [backend]
      healthcheck:
        command: ["/app/healthcheck"]
        interval: 5s
        timeout: 3s
        retries: 20

  tasks:
    migrate:
      phase: migration
      image: app
      command: ["/app/migrate"]
      dependsOn:
        - target: db
          condition: healthy
      rerun: oncePerDeployment
      timeout: 5m

  package:
    includeRuntime: false
    embedImages: false
    generateReadme: true
    generateEnvExample: true

  x-ui:
    nodes:
      app: {x: 480, y: 120}
      db: {x: 120, y: 120}
```

### 9.4. Root fields

| Field | Required | Semantics |
|---|---:|---|
| `apiVersion` | yes | Schema dispatch. |
| `kind` | yes | В v1 только `Project`. |
| `metadata.name` | yes | Stable project ID и Compose project-name prefix. |
| `metadata.version` | yes | Valid SemVer без префикса `v`. |
| `metadata.description` | no | Public documentation; секреты запрещены. |
| `spec.target.profile` | yes | В v1 только `compose-docker/v1`. |
| `spec.mode` | yes | `development` или `release`. |
| `spec.parameters` | no | Несекретные typed values. |
| `spec.secrets` | no | Только declarations, никогда values. |
| `spec.images` | yes | OCI images или development builds. |
| `spec.builds` | no | Сборка статики пакетным менеджером на `iacpack build` (§9.5.1). |
| `spec.resources` | no | Явно включаемые файлы. |
| `spec.networks` | no | Managed Compose networks. |
| `spec.volumes` | no | Managed named volumes. |
| `spec.services` | yes | Не менее одного service. |
| `spec.databases` | no | Дополнительная база на уже существующем сервисе `on`. Compile порождает задачу `db-<id>` (`phase: migration`) после `healthy`, применяет SQL из `migrations` по порядку имён и подставляет DSN в `urlParameter`. Пароль остаётся ссылкой на секрет. Второго сервера базы нет. |
| `spec.tasks` | no | One-shot container lifecycle tasks. |
| `spec.package` | no | Packaging options. |
| `spec.x-ui` | no | UI-only metadata, исключённая из deployment semantic hash. |

### 9.5. Images и build contexts

Image declaration является discriminated union:

1. `ref`: OCI reference. В `release` MUST содержать digest.
2. `build`: development-only Dockerfile context.

Build context:

- не запускается во время `validate`, `lock` или `build`;
- файлы задаются explicit include/exclude rules;
- exact expanded file list и hashes фиксируются в lockfile;
- Dockerfile не переписывается;
- применение требует interactive confirmation или `--allow-development-build --yes`;
- runtime запускает build с explicit context/Dockerfile/args и проверяет полученный image ID, если указан `expectedImageID`;
- network для Docker build по умолчанию отключён; его включение требует compile policy и отдельного high-risk confirmation;
- результат Docker build не считается reproducible, если отсутствует independently verified `expectedImageID`.

В `release` mode mutable tags и local build являются ошибкой. Development build declaration имеет точную форму:

```yaml
images:
  app:
    build:
      context: app
      dockerfile: Dockerfile
      include: ["**"]
      exclude: [".git/**", ".env", "dist/**"]
      args:
        APP_MODE: {literal: development}
      expectedImageID: "sha256:<64-lowercase-hex>" # optional
```

`context` и `dockerfile` — WorkspacePath, Dockerfile обязан находиться внутри context. Build args принимают только literal/build-parameter value sources и не могут содержать secrets.

### 9.5.1. Package builds (frontend)

`spec.builds` — сборка статики менеджером пакетов на машине `iacpack build`. `apply` только монтирует уже собранный payload и не запускает пакетный менеджер.

```yaml
builds:
  spa:
    context: shared/web/frontend
    packageManager: pnpm   # pnpm | npm | yarn | bun
    install: frozen
    script: build          # имя script из package.json
    output: dist
    entry: index.html      # по умолчанию
services:
  web:
    mounts:
      - build: spa
        target: /usr/share/nginx/html
        readOnly: true
```

- Нет `package.json` в `context` — ошибка `BUILD.FRONTEND` на validate/build.
- `packageManager` обязателен. Неизвестный менеджер — ошибка. Lockfile не выбирает менеджер.
- `install: frozen` требует lockfile менеджера (`pnpm-lock.yaml`, `package-lock.json`, `yarn.lock`, `bun.lock` или `bun.lockb`).
- Команда — `<manager> run <script>`, не произвольный shell.
- Сборка идёт во временной копии. Каталоги вне `context`, на которые ссылаются `pnpm-workspace.yaml` или importer в lockfile (`../../packages/…`), копируются с тем же относительным путём. Относительный импорт исходника, который выходит из `context` и указывает на существующий файл в workspace, копируется так же. Путь вне workspace или отсутствующий importer — ошибка `BUILD.FRONTEND` с этим путём, до `pnpm install`. Исходный workspace не меняется. `install` может использовать сеть. Каталог сборки pnpm создаётся на том же диске, что и store; если это невозможно, install идёт с `--package-import-method=copy`.
- Пустой `output` или отсутствие непустого `entry` — ошибка build. Пустой прошлый output кэшем не считается.
- Результат лежит в payload `build-<id>/` и монтируется как `./payload/build-<id>`.

### 9.6. Parameters

Parameter имеет:

- `phase`: `build` или `deploy`;
- `type`: `string`, `integer`, `number`, `boolean`, `enum`;
- optional non-secret `default`;
- constraints (`minimum`, `maximum`, `pattern`, `choices`);
- public description.

Источники в порядке приоритета:

1. explicit CLI `--set name=value` для несекретных значений;
2. explicit `--values path`;
3. manifest default.

Ambient environment, `.env` и shell expansion не читаются. Синтаксис `${VAR}` не интерполируется. В полях используются tagged value sources:

```yaml
literal: value
parameter: parameter-id
secret: secret-id
```

`secret` называет id из `spec.secrets` и не хранит значение. `plan`/`apply` получают файлы через `--secrets-dir <dir>` (имя файла = id) или `--secret-file id=/path`. Явная пара заменяет тот же id из каталога.

Build parameters разрешаются до compilation и входят в lock/provenance. Deploy parameters разрешаются при `plan/apply` и не изменяют digest исходного bundle; hash resolved deployment фиксируется в journal.

### 9.7. Secrets

- Manifest содержит только ID, description, required и delivery.
- В v1 поддерживается только `delivery: file`.
- Значение передаётся через `--secrets-dir <dir>` (имя файла = id) или `--secret-file id=/path`. Явная пара заменяет тот же id из каталога. `--secret id=value`, environment values и secret CLI arguments запрещены.
- Secret file копируется в private runtime directory `secrets/` (`0700`, файл `0440`). Rendered Compose монтирует его как `../secrets/<id>` относительно `rendered/compose.yaml`. Нет файла — ошибка до `compose up` / `compose run`, не ошибка движка `statfs`. Числовой `user`, кроме `0` и `0:0`, перед стартом получает `podman unshare chown` этого файла. На SELinux enforcing каталог помечается `container_file_t`.
- Ссылка на секрет в argv (`{secret: id}` или элемент `parts`) и в склейке env (`{parts: [literal, secret, literal]}`) рендерится как `${IACPACK_SECRET_<ID>}`. Значение в манифест, lock, bundle и process arguments iacpack не пишется; движок подставляет его из окружения apply.
- Secret values не записываются в manifest, lock, bundle, plan, journal, logs, diagnostics, README, Compose environment или process arguments.
- Redaction является defense-in-depth, а не обещанием распознать неизвестный секрет.
- `.env.example` содержит только placeholders и безопасные non-secret defaults. Populated `.env` никогда не создаётся и не пакуется.

### 9.8. Services

Service поддерживает:

- image reference;
- `command` и `entrypoint` только как argv: непустая строка, `{literal: ""}` (пустой аргумент, например `--logfile ""`), `{secret: id}`, либо `{parts: [...]}` из строк, `{parameter}` и `{secret}`;
- environment: `{literal}`, `{parameter}`, `{secret}` или `{parts}` из этих трёх (склейка URL с паролем);
- secret file mounts;
- ports;
- named volumes, declared resources и mount `build: <id>` на payload `spec.builds`;
- managed networks;
- healthcheck;
- restart policy из allowlist;
- optional `user` (`uid`, `uid:gid` или имя);
- resource limits из supported Compose subset;
- readiness dependencies.

Privileged mode, host PID/IPC/network, произвольные devices (включая CDI и `device_ids`), Docker socket, arbitrary capabilities и undeclared host binds запрещены. Единственное device-поле v1 — `resources.gpu` (целое 1..8, строка `all` или `devices-only`): `all` и число рендерятся как Compose `deploy.resources.reservations.devices` с `driver: nvidia`, `count` и `capabilities: [gpu]` (ADR-021). `gpu: all` без CDI `nvidia.com/gpu=all` останавливает apply до старта контейнеров (`APPLY.GPU_UNAVAILABLE`). `gpu: devices-only` требует `ldLibraryPath`, монтирует host `/dev/nvidia*` и выставляет `NVIDIA_VISIBLE_DEVICES` в индексы этих карт (`nvidia0` → `0`). Значение `void` скрывает смонтированные карты от CUDA. При включённом SELinux тот же сервис получает `security_opt: label=disable`, потому что `container_t` не может открыть узлы `xserver_misc_device_t`.

### 9.9. Ports

Port record включает:

- container port;
- optional host port как literal/parameter;
- bind IP;
- protocol `tcp`/`udp`.

Compile-time validation проверяет literal ports и гарантированно равные parameter references. После разрешения deploy parameters та же проверка повторяется в `plan`.

Конфликт существует при одинаковых host port и protocol, если bind-адреса равны или перекрываются. `0.0.0.0` перекрывает любой IPv4 address; `::` консервативно считается перекрывающим любой IPv4/IPv6 bind, так как v1 не моделирует `ipv6Only`. Разрешены только IP literals, hostname запрещён. Одинаковые container ports разных services допустимы. Занятость порта другим host process проверяется в preflight и не может считаться полностью race-free.

### 9.10. Networks и volumes

- Networks и volumes являются отдельными resource classes, а не service types.
- Имена provider resources derive из project instance ID и logical ID.
- Все создаваемые resources получают ownership labels.
- Volume retention: `retain` по умолчанию; `delete` только при явной declaration и отдельном destructive confirmation.
- `destroy --delete-volumes` удаляет только managed volumes с `retention: delete`; `retain` volumes `iacpack` не удаляет даже этим флагом.
- External volumes/networks в v1 являются references `{external: {name: "..."}}`, никогда не создаются/удаляются и требуют allowlist в compile policy и operator policy.

### 9.11. Dependencies, health и graph

- Edge направлен от prerequisite к dependent.
- `condition`: `started`, `healthy` или `completedSuccessfully`.
- `healthy` требует healthcheck.
- Readiness/task execution graph MUST быть DAG.
- Cycle diagnostic MUST показывать конкретный cycle path.
- Независимые nodes сортируются лексикографически по stable ID.
- Сетевое общение между services не обязано описываться dependency edge.

Разрешённая phase partial order:

| Dependent | Допустимый prerequisite |
|---|---|
| `preStart` task | другой `preStart` task |
| service | `preStart` task, service (`started`/`healthy`), или `migration` task (`completedSuccessfully` только) |
| `migration` task | service или `migration` task |
| `postDeploy` task | service, `migration` task или `postDeploy` task |

`completedSuccessfully` разрешён только для task той же или более ранней phase. `preStart` не может зависеть от service; service не может зависеть от `postDeploy`; service→migration только с `completedSuccessfully`; migration не может зависеть от postDeploy. Image availability, managed networks/volumes и declared payload автоматически являются prerequisites соответствующих container actions.

### 9.12. Lifecycle tasks

Task выполняется one-shot container, а не host shell.

Поля:

- `phase`: `preStart`, `migration`, `postDeploy`;
- `image`;
- `command` argv array;
- optional `entrypoint` argv. Он заменяет `ENTRYPOINT` образа. Пустой массив очищает его, и `command` становится исполняемым файлом. Без поля остаётся entrypoint образа;
- optional `user`: uid (`0`), `uid:gid` или имя;
- optional working directory;
- dependencies;
- timeout;
- retry count/backoff;
- `rerun`: `always`, `oncePerDeployment`, `manual`;
- mounts/networks/secrets по тем же правилам, что service.

Task рендерится сервисом с `profiles: [iacpack-task]` и `restart: no`, поэтому обычный `compose up` его не стартует. `apply` и `iacpack task run` вызывают `compose --profile iacpack-task run --rm --no-deps <task>`. Без `--profile` podman-compose сообщает, что сервиса нет.

Каждый `oncePerDeployment` ключуется хешем `command`, `entrypoint`, `user` и байтов файлов mount (payload bundle или host-путь), без байтов секретов. Смена скрипта на монтировании перезапускает только эту task, даже если digest bundle не изменился; совпавший хеш её не перезапускает. Repack, docs и `x-ui` сами по себе task не запускают. `manual` запускается только командой `iacpack task run`. Default failure policy v1 — `stop`. Произвольный `continue` и автоматический rollback irreversible tasks не поддерживаются. Автор task отвечает за её application-level idempotency. Для `spec.databases` повторно исполняется SQL-файл, у которого сменился SHA-256; неизменный файл пропускается, и повторный запуск должен быть безопасен.

### 9.13. Нормативный data dictionary v1alpha1

JSON Schema является точным machine-readable contract, но реализация MUST соблюдать следующие defaults и limits:

| Object/field | Contract |
|---|---|
| `metadata` | `name`, `version` required; `description` ≤ 4 KiB. |
| `parameter.phase` | required: `build` или `deploy`. |
| `parameter.type` | required; `enum` дополнительно требует 1–100 unique scalar `choices`. |
| `parameter.pattern` | RE2, ≤ 1 KiB; catastrophic/backtracking regex engines запрещены. |
| `secret` | `required` default `true`; `delivery` только `file`; description ≤ 4 KiB. |
| `image.ref` | `ref`, `delivery: pull`; digest обязателен в release. |
| `image.build` | shape из §9.5; только development mode. |
| `builds.<id>` | `context`, `packageManager` (`pnpm`\|`npm`\|`yarn`\|`bun`), `install: frozen`, `script`, `output`; `entry` default `index.html`. §9.5.1. |
| `service.user` / `task.user` | optional uid (`0`), `uid:gid` или имя; пустое оставляет `USER` образа. |
| `task.entrypoint` | optional argv. Заменяет `ENTRYPOINT` образа. `[]` очищает его, `command` становится исполняемым файлом. |
| `argv` | непустая строка, `{literal}` (в том числе `""`), `{secret}`, или `{parts}` из строк / `{parameter}` / `{secret}`. |
| `resource` | `source`, non-empty `include`; optional `exclude`, `destination`, `mode`, `allowEmpty=false`. |
| `network` | либо managed `{}`, либо external reference; union arms не смешиваются. |
| `volume` | managed `{retention: retain|delete}` либо external reference; default `retain`. |
| `service.environmentFrom` | optional service id. Env этого сервиса копируется, затем `environment` переопределяет отдельные ключи. Цикл и неизвестный id — ошибка. `plan` перечисляет ключи, которых нет у сервиса с тем же image (`PLAN.ENV_MISSING`). |
| `service.restart` | `no`, `always`, `on-failure`, `unless-stopped`; default `unless-stopped`. |
| `service.resources.cpus` | decimal string `0.001..1024`; без floating serialization. |
| `service.resources.memory` | integer bytes `1 MiB..1 TiB`. |
| `healthcheck` | executable argv `command` (shell/CMD-SHELL запрещён); `interval`, `timeout`, optional `startPeriod`; `retries` 1–100. |
| duration | ASCII decimal + `ms|s|m|h`; > 0, максимум 24h. |
| retry | count 0–10; initial backoff 100ms–5m; multiplier 1–10; max backoff ≤ 5m. |
| `x-ui.nodes.<id>` | finite `x`, `y`, optional category `app|database|proxy|cache|custom`. |
| `x-ui.viewport` | finite `x`, `y`, `zoom` 0.1–4; optional. |

Map keys, union arms, defaults и `additionalProperties` MUST быть полностью зафиксированы в checked-in schema corpus. Поля, отсутствующие в этом документе и JSON Schema, не поддерживаются.

### 9.14. Package options

- `generateReadme` и `generateEnvExample` default `true`; при `false` соответствующий optional entry отсутствует.
- `includeRuntime` default `false`, допустим только для `--format distribution`; distribution требует `true`.
- `embedImages` default `false`, допустим для bundle/distribution и требует explicit verified OCI image cache.
- CLI output format не может молча переопределить противоречащую manifest option; возвращается `MANIFEST.PACKAGE_OPTION_CONFLICT`.

### 9.15. Compile policy и operator policy

`iacpack-policy.yaml` имеет versioned schema `iacpack.policy/v1`. Compile policy:

- задаёт allowed OCI registries;
- разрешает/запрещает development builds, build network и external resource references;
- задаёт resource/security ceilings и package quotas;
- может выбрать только capability, явно поддерживаемую v1 schema/provider, и не может включить отсутствующую/unsupported capability.

Effective compile policy (explicit file или checked-in immutable default) применяется в `validate/lock/build`; canonical digest фиксируется в lock и bundle.

Immutable default разрешает registries `docker.io`, `ghcr.io`, `quay.io`, development builds только в development mode, запрещает build network и external networks/volumes и использует quotas §13.2. Explicit compile policy может задать другой registry allowlist и включить поддерживаемую high-risk capability; каждое такое требование фиксируется в plan/bundle. Operator policy должен независимо разрешить её и всегда может запретить или уменьшить limit.

Operator policy передаётся локально в `plan/apply`, может быть только строже compile policy и не входит в bundle как доверенное решение. Bundle может объявлять required capabilities, но не может ослабить operator policy. Unsupported privileged/host/arbitrary-device/socket capabilities отсутствуют в v1 schema и не могут быть «разрешены exception-файлом». `resources.gpu` — узкое исключение ADR-021, не путь устройства и не CDI.

## 10. Path и resource model

### 10.1. Типы путей

- **WorkspacePath:** относительный путь от корня workspace. Корень — каталог манифеста, кроме черновика в `<workspace>/imports/<file>`: тогда корень — родитель `imports` (тот же, что `--workspace` у `import propose`).
- **BundlePath:** относительный controlled path внутри bundle.
- **ContainerPath:** абсолютный POSIX path внутри контейнера.
- **ManagedHostPath:** path под state root, создаваемый runtime.
- **ExternalHostPath:** операторский capability, запрещённый default policy.

### 10.2. Правила

- WorkspacePath не может быть абсолютным, пустым, содержать NUL, backslash, drive prefix или `..`.
- После canonical resolution вход обязан оставаться внутри workspace.
- Devices, sockets и FIFOs во входах запрещены. Hardlink (nlink > 1) принимается как обычный файл; в бандле хранится содержимое, не отдельный тип ссылки.
- Symlink принимается, только если цель после разрешения остаётся внутри workspace. Symlink, который выходит из workspace, запрещён.
- Один и тот же workspace path может входить в несколько resources или build context. В lock/bundle он записывается один раз. Case-folding и Unicode-normalization collisions различных путей остаются ошибкой.
- Include выбирает входы. Обход не заходит в каталоги, которые include не может задеть. Source-файл — один файловый input, не обход каталога.
- Include globs используют `/`, имеют документированную doublestar-семантику и сортируются по UTF-8 bytes.
- Empty include match является ошибкой, если явно не задан `allowEmpty: true`.
- Output directory никогда не может стать входом.
- Необъявленные `.env`, `.git`, private keys, previous outputs и editor files не включаются.
- Source permissions нужны только для чтения.
- Arbitrary text scanning и path rewriting запрещены.

Discovery реализуется в `init --discover` и `inspect --discover` (и `GET /api/v1/discover`). Поиск ограничен depth 3. Имена (P17 / ADR-017): `Dockerfile`, `Dockerfile.*`, `compose.yaml`, `compose.yml`, `docker-compose.yml`, `docker-compose.yaml`, `compose.*.yml`, `compose.*.yaml`, `docker-compose.*.yml`, `docker-compose.*.yaml`, `stack*.yml`, `stack*.yaml`, `Caddyfile`, `nginx.conf`. Hidden directories и output roots пропускаются. Он никогда не читает/предлагает `.env`, keys, credential stores или secret-like files. Результат детерминирован, reviewable и никогда молча не меняет build inputs.

Отдельно от discovery: `import propose` (CLI) и `POST /api/v1/import/propose` строят **черновик** Project из workspace-relative Compose (primary, один или несколько путей — ADR-017) и/или Dockerfile (secondary). Propose никогда не пишет диск. UI Accept заменяет только in-memory Project; запись на диск — только explicit Save/Save As. Подмножество полей — inverse compose-docker/v1 emit плюс external networks/volumes (ADR-016). Матрица Compose Specification ↔ import ↔ manifest ↔ render, severity partial Accept и фазы реализации — **ADR-020** и `docs/reference/compose-import-fidelity.md`. Unsupported/policy-denied поля и содержимое `env_file`/secret-like/`privileged`/host binds дают diagnostics/refusals и не попадают в draft молча. `${VAR}` становится parameter stub без значений; ambient `.env` не читается (в отличие от `docker compose config` — ADR-020 §2).

P18 / ADR-018: onboarding может предлагать **declarations** секретов и gaps параметров без значений; MAY обновлять только `.env.example` placeholders. Populated `.env` MUST NOT создаваться или упаковываться.

P19 / ADR-019: явная authoring-команда MAY записывать opted-in GitHub CI/release helper templates в workspace. Автоматическая публикация в GitHub, `gh` push/tag/release из приложения остаются запрещены (§5).

## 11. Lockfile

`iacpack.lock.json` — canonical JSON по RFC 8785. Он содержит:

- format version;
- canonical manifest SHA-256;
- schema digest;
- provider/renderer version;
- compiler version;
- build-mode и non-secret build parameter values;
- каждый включённый file path, size, normalized mode и SHA-256;
- resolved OCI references/digests;
- canonical compile-policy digest;
- generation metadata, не влияющую на deterministic content либо нормализованную.

`iacpack lock` — единственная команда, которая обновляет lockfile. Разрешение mutable image tag выполняется только с `--update-images` и явным network access. `build` по умолчанию работает как `--locked` и завершается ошибкой при drift.

Lockfile не содержит timestamp, username, hostname, absolute workspace path или secret data.

Начиная с реализации input resolver обычный `validate` выполняет полную schema, semantic, path, input и policy validation. `validate --syntax-only` явно ограничивает проверку parser/schema и не может использоваться как release gate.

## 12. Compilation и validation pipeline

### 12.1. Pipeline

1. Безопасно прочитать bytes.
2. Построить YAML AST/JSON tree.
3. Проверить parser restrictions.
4. Определить schema version.
5. Выполнить JSON Schema validation.
6. Преобразовать в typed authoring model.
7. Разрешить explicit build parameters.
8. Разрешить и проверить paths/resources.
9. Выполнить semantic/provider/policy validation.
10. Построить immutable IR.
11. Построить deterministic action graph.
12. Сформировать virtual artifact tree.
13. Выполнить secret/output policy scan.
14. Материализовать staging output.
15. Повторно проверить hashes/schema.
16. Atomic rename файла в target либо commit нового directory target.

До шага 14 source и target не изменяются. Bundle/lock/manifest files используют same-filesystem atomic rename. Directory build в v1 требует, чтобы target не существовал; overwrite/replace directory не поддерживается. При ошибке staging очищается; ранее существующий output никогда не изменяется.

### 12.2. Уровни validation

- Syntax/parser restrictions.
- Schema/types/unknown fields.
- Referential integrity.
- Paths, globs, collisions и permissions.
- Parameter resolution.
- Secret placement rules.
- Image/release immutability.
- Port/network/volume conflicts.
- Dependency/task DAG.
- Provider capability compatibility.
- Security policy.
- Bundle/archive integrity.
- Runtime preflight.

### 12.3. Diagnostics

Diagnostic schema:

```json
{
  "code": "MANIFEST.PATH_OUTSIDE_WORKSPACE",
  "severity": "error",
  "message": "Resource path escapes the workspace root",
  "source": {
    "file": "iacpack.yaml",
    "pointer": "/spec/resources/config/source",
    "line": 18,
    "column": 11
  },
  "hint": "Use a path relative to the manifest directory",
  "retryable": false
}
```

Требования:

- stable uppercase namespaced codes;
- `error`, `warning`, `info`;
- source location и JSON Pointer, если применимо;
- deterministic order: file → pointer → code;
- aggregate independent validation errors;
- никаких secret values/raw credential-bearing provider responses;
- stack traces только при explicit debug и только в private logs.

Начальные namespaces: `CLI`, `MANIFEST`, `SCHEMA`, `INPUT`, `LOCK`, `GRAPH`, `POLICY`, `BUNDLE`, `PROVIDER`, `RUNTIME`, `INTERNAL`.

### 12.4. Machine-output envelope

Каждая команда с `--output json` соответствует checked-in schema `schemas/cli/v1/<command>.schema.json` и выдаёт ровно один document:

```json
{
  "schemaVersion": "iacpack.cli/v1",
  "command": "validate",
  "ok": false,
  "result": null,
  "diagnostics": []
}
```

Envelope не содержит current timestamp. Поля `schemaVersion`, `command`, `ok`, `result`, `diagnostics` обязательны; command-specific `result` имеет отдельную schema. `version` использует тот же envelope и возвращает product, version, Go version, commit и dirty flag.

## 13. Artifact и bundle contract

### 13.1. Canonical bundle

Формат v1 — deterministic `tar.gz` с расширением `.iacbundle`.

```text
bundle.json
manifest.canonical.json
deployment.json
iacpack.lock.json
policy.compile.json
rendered/compose.preview.yaml
payload/<resource-id>/...
images/oci-layout/...                 # только offline profile
docs/README.md
docs/architecture.mmd
config/.env.example
```

`rendered/compose.preview.yaml` присутствует только если все deploy parameters имеют non-secret defaults. Финальный Compose всегда materialize при apply typed serializer-ом.

`bundle.json` содержит:

- `formatVersion`;
- project identity/version;
- compiler/provider/schema/policy identifiers;
- hashes и sizes всех остальных entries;
- required runtime version;
- bundle capabilities;
- optional signature metadata reference.

`bundle.json` не хеширует сам себя. Detached signature не меняет bytes bundle.

`policy.compile.json` — canonical effective compile policy, digest которой совпадает с lockfile. `validate`, `lock` и `build` принимают `--policy <path>`; без флага используется immutable default. `plan` и runtime commands принимают отдельный `--operator-policy <path>`.

### 13.2. Determinism

- Paths приводятся к Unicode NFC; entries сортируются по UTF-8 bytes normalized path.
- Duplicate/archive traversal/absolute entries запрещены.
- Tar format — POSIX USTAR; path/segment limits валидируются до записи.
- Tar `mtime` = Unix epoch `1970-01-01T00:00:00Z`; atime/ctime отсутствуют.
- UID/GID = 0; owner/group names пустые.
- Directory mode `0755`; regular file `0644`; executable только по declaration.
- Line endings generated text — LF, encoding — UTF-8 без BOM.
- Gzip DEFLATE level 9; header mtime = Unix epoch, OS = 255, Name/Comment/Extra пустые.
- Никаких cwd, temp path, locale, timezone, hostname, username, umask, random UUID и current time.
- Reproducibility scope: одинаковые input bytes, lockfile, build parameters, policy, renderer и tool version.

Default verifier limits: 50 000 entries, 4 GiB на entry, 20 GiB суммарного expanded content, compression ratio 200:1 и path ≤ 255 UTF-8 bytes. Compile policy может только уменьшить defaults. Hard cap: 100 000 entries и 100 GiB; превышение hard cap не может быть разрешено policy.

### 13.3. Output strategies

1. **Directory:** inspectable deterministic tree.
2. **Online bundle:** images загружаются по digest во время apply.
3. **Offline bundle:** OCI layouts вложены и проверяются по digest.
4. **Linux distribution:** bundle + platform runtime + thin `install.sh`.

Название `self-contained` допустимо только для offline distribution, содержащего runtime и все OCI images.

Offline workflow разделён:

1. `iacpack images fetch --file iacpack.yaml --lock iacpack.lock.json --output image-cache/` явно использует сеть и создаёт verified OCI layout/cache.
2. `iacpack build --embed-images --image-cache image-cache/` не использует сеть и принимает только blobs, совпадающие с lockfile digests.

### 13.4. Installer wrapper

`install.sh`:

- не содержит orchestration business logic;
- находит bundled `bin/iacpack`;
- проверяет его checksum;
- передаёт аргументы в `iacpack apply`;
- не вызывает `sudo`;
- использует safe quoting и `set -eu`;
- проходит ShellCheck;
- одинаков при одинаковом runtime contract.

## 14. CLI contract

### 14.1. Команды

```text
iacpack init
iacpack validate
iacpack lock
iacpack images fetch
iacpack build
iacpack verify <bundle>
iacpack render compose <bundle>
iacpack inspect manifest|inputs|graph|bundle
iacpack diff --from <source> --to <source>
iacpack schema export
iacpack manifest migrate
iacpack doctor
iacpack plan <bundle-or-kit> [--workspace <dir>]
iacpack apply <bundle-or-kit> [--instance <id>] [--workspace <dir>]
iacpack bundle --workspace <dir> --output-dir <dir> [--policy <path>] [--embed-local-images]
iacpack explain <bundle> <service>
iacpack status <project> [--instance <id>]
iacpack resume <project> [--instance <id>] [--dry-run]
iacpack task run <project> <task> [--instance <id>]
iacpack destroy <project> [--instance <id>] [--dry-run]
iacpack serve --workspace <path>
iacpack version
```

`iacpack bundle` пишет каталог комплекта и `project.iacbundle`. Пустой `host-paths.json` и образ `local-only` без `--embed-local-images` — предупреждения. `plan` и `apply` каталога останавливаются до контейнеров одной диагностикой `PORTABLE.HOST_PATH` (пустые и неабсолютные пути) и одной `PORTABLE.LOCAL_IMAGE` (незагруженные `local-only` и `load`). После проверки они берут `project.iacbundle`. Заполненное `value` подменяет host bind этого сервиса или задачи, в том числе литерал из манифеста. `--set` того же параметра остаётся как задал оператор.

`--instance` default — `default`; instance ID соответствует `^[a-z][a-z0-9-]{0,30}$`. `apply` нового bundle в тот же `(project, instance)` является update. Project берётся только из уже verified bundle; для state-only commands задаётся positional argument.

Provider project slug детерминирован: `iacp-` + первые 40 ASCII-символов `<project>-<instance>` + `-` + первые 12 lowercase hex SHA-256 от `project + NUL + instance`. Он не меняется между updates и не зависит от path.

### 14.2. Side-effect matrix

| Command | Writes workspace | Network | Executes project/provider |
|---|---:|---:|---:|
| `init` | explicit new files | no | no |
| `validate` | no | no | no |
| `inspect` | no | no | no |
| `lock` | lockfile only | only explicit image resolution | no project code |
| `images fetch` | explicit cache only | yes | registry read-only |
| `build` | output only | no | no |
| `verify` | no | no | no |
| `render` | explicit output only | no | no |
| `diff` | no | no | no |
| `doctor` | no | provider queries | provider read-only |
| `plan` | no | no by default | no mutation |
| `bundle` | kit directory and `project.iacbundle` | no | `save` only with `--embed-local-images` |
| `explain` | no | no | no |
| `apply` | state/runtime | image pull allowed | yes |
| `resume` | state/runtime | as required | yes |
| `task run` | state/runtime | as required | explicit one-shot task |
| `destroy` | state/runtime | no by default | destructive |
| `serve` | manifest only on explicit save | no | no deploy |

### 14.3. Global output rules

- Default human diagnostics идут в stderr.
- Machine result идёт в stdout.
- `--output json` выдаёт один versioned JSON document.
- ANSI/progress отключены для non-TTY и JSON.
- `--quiet` не скрывает errors.
- `--debug` не отменяет secret redaction.
- Команды поддерживают cancellation; exit code `130`.

### 14.4. Exit codes

| Code | Meaning |
|---:|---|
| 0 | Success |
| 2 | Invalid CLI invocation |
| 3 | Manifest/values/policy invalid |
| 4 | Input/output filesystem or build failure |
| 5 | Bundle integrity/signature failure |
| 6 | Provider/preflight failure before mutation |
| 7 | Partial apply; recovery required |
| 70 | Internal invariant failure |
| 130 | Interrupted |

### 14.5. `diff`

`diff` всегда получает два explicit источника: manifest, lock, bundle или recorded deployment state. Он сравнивает canonical semantics, игнорирует YAML formatting и отдельно показывает:

- service/image changes;
- parameter schema changes;
- resource file changes;
- network/volume changes;
- task graph changes;
- destructive/irreversible risk.

Неявного «предыдущего manifest» не существует.

Recorded deployment state становится допустимым diff source только после создания state runtime-ом; compiler-only releases обязаны вернуть `CLI.UNSUPPORTED_SOURCE`, а не угадывать baseline.

## 15. Deployment runtime

### 15.1. Apply stages

1. Verify archive structure, hashes, local signature evidence и bundle metadata во временном read-only context.
2. Derive verified project ID и explicit/default instance ID.
3. Acquire per-instance operation lock.
4. Validate runtime parameters, secret file references и effective operator policy.
5. Build deterministic deployment plan. Если черновик workspace (`--workspace` или `./iacpack.yaml` того же project) новее lock внутри bundle, plan и apply останавливаются с текстом `lock устарел` и командой `iacpack diff --from <manifest> --to <lock или bundle>`. Мутации нет.
6. Display plan; получить confirmation, если не `--yes`.
7. Provider preflight без mutation.
8. Create run ID и transaction journal.
9. Materialize private runtime directory и final Compose.
10. Provision managed networks/volumes/directories.
11. Pull/load/build approved images.
12. Execute `preStart` tasks.
13. Start services в deterministic order (`compose up -d --no-deps --force-recreate <service>` на сервис).
14. Wait bounded health/readiness (engine `ps`/`inspect` labels; не Docker-only `compose ps --status/--services`). Сбой start/ready одного сервиса не останавливает независимые последующие действия; диагностика именует сервис (и CDI device для GPU).
15. Execute migration tasks.
16. Execute post-deploy tasks.
17. Record successful active bundle and checksums.

Все process calls используют argv arrays. Runtime очищает ambient `COMPOSE_*`, `DOCKER_HOST`, `DOCKER_CONTEXT` и не допускает auto-loaded `.env`/override files. V1 использует только local Unix socket `/var/run/docker.sock` либо explicit `--docker-socket` local Unix path; TCP/SSH endpoints запрещены. Private registry config принимается только через explicit `--docker-config` directory, проверяется и не копируется в state.

### 15.2. State

Default user state:

```text
${XDG_STATE_HOME:-$HOME/.local/state}/iacpack/projects/<project>/<instance>/
  state.json
  journal.db
  rendered/compose.yaml
  payload/
  secrets/
  logs/
```

- State schema версионируется.
- Secret values отсутствуют в structured state/journal/logs; единственное допустимое место — designated `secrets/` payload directory.
- Secret source обязан быть regular non-symlink file ≤ 1 MiB и открывается с no-follow semantics. Managed copy хранится `0440` под directory `0700`, атомарно обновляется на apply, используется при resume и удаляется при destroy. Secure physical erase на SSD не обещается.
- Logs имеют mode `0600`, rotation: максимум 10 файлов по 10 MiB.
- Concurrent operations одного instance запрещены lock-ом.
- Managed Docker resources имеют labels project, instance, bundle digest, run ID и owner.
- State migration перед изменением создаёт verified backup metadata; rollback относится только к metadata schema, не к provider side effects.

### 15.3. Failure и recovery

Общая транзакционность не обещается:

- image pulls, containers, volumes и external systems являются side effects;
- migrations могут быть необратимыми;
- task может частично завершиться;
- host port preflight имеет race.

Runtime обеспечивает:

- mutation-free preflight;
- durable journal до и после каждого action;
- cleanup только ресурсов, созданных текущим run и помеченных reversible;
- никогда автоматически не удаляет named volumes, host files и pre-existing resources;
- commit barrier перед первой irreversible task;
- `unknown` state при interruption внутри action;
- probe/reconcile на `resume`;
- конкретные команды recovery в diagnostic.

Это называется **resumable deployment with compensating cleanup**, а не atomic rollback.

### 15.4. Re-apply и update

- Повторный apply того же bundle — no-op кроме explicitly `always` tasks.
- Новый bundle сначала diff-ится с active state.
- Destructive changes требуют confirmation.
- Persistent volumes сохраняются.
- Image update выполняется по digest.
- Failed update не объявляется successful active version.
- `manual` task запускается только `task run` из active verified deployment, journaled тем же runtime и требует confirmation для irreversible task.
- `status` показывает active bundle/deployment semantic hash, health summary, last run, pending/unknown recovery actions и owned resource IDs без credentials.

### 15.5. Destroy

- По умолчанию удаляются только managed containers и networks.
- Volumes сохраняются.
- `--delete-volumes` удаляет только managed volumes, объявленные `retention: delete`, и требует interactive confirmation.
- В non-interactive mode требуется одновременно `--delete-volumes --yes`.
- Volumes с `retention: retain` никогда не удаляются `iacpack`; manual provider cleanup остаётся ответственностью operator.
- Ресурсы без matching ownership labels не удаляются.

## 16. Docker Compose renderer/provider

- YAML создаётся typed serializer-ом, не string concatenation.
- Поддерживается документированный Compose subset.
- Generated Compose проходит internal schema validation.
- Integration tests запускают `docker compose config --quiet` на minimum и latest supported versions.
- Project name задаётся явно и стабильно.
- Paths в Compose указывают только на managed deployment root.
- Не используются implicit override files/profiles.
- `depends_on` не считается readiness без health condition.
- Provider errors преобразуются в structured diagnostics; raw stderr хранится только после redaction.

## 17. Web UI и local API

### 17.1. Web UI

Функции v1.1:

- import/load manifest;
- propose Compose/Dockerfile → reviewable draft Project → Accept (in-memory) → explicit Save (ADR-016);
- multi-file Compose propose, widened discover, structured refusals (ADR-017);
- secrets/parameters onboarding stubs without values; `.env.example` placeholders only (ADR-018);
- explicit GitHub CI/release template generation without auto-publish (ADR-019);
- service/resource canvas;
- typed dependency edges;
- selection-based property panel;
- full schema-known editors for parameters, secret declarations, images, resources, services, ports, mounts, healthchecks, networks, volumes, tasks, package и `x-ui`;
- live diagnostics;
- Compose preview и artifact tree;
- explicit Save/Save As;
- semantic export;
- graph auto-layout;
- keyboard operation и accessible forms.

UI labels `app`, `database`, `proxy`, `cache` — только presentation metadata и не меняют runtime semantics.

`x-ui` хранит node positions и UI preferences. Изменение только `x-ui` не меняет deployment semantic hash или generated infrastructure.

### 17.2. Synchronization

- Manifest не перезаписывается при каждом UI keystroke.
- Save использует revision hash/ETag и `If-Match`.
- External edit вызывает reload/merge conflict, а не silent overwrite.
- Invalid external YAML остаётся на диске; UI сохраняет last valid visual model и показывает diagnostics.
- Canonical save может нормализовать formatting; UI сообщает об этом.
- Semantic round-trip обязателен. Сохранение comments не обещается в v1.1.

### 17.3. Local API

Минимальные endpoints:

```text
GET  /api/v1/health
GET  /api/v1/schema/manifest
GET  /api/v1/manifest
PUT  /api/v1/manifest
POST /api/v1/manifest/save-as
POST /api/v1/export/manifest
POST /api/v1/validate
POST /api/v1/inspect/graph
POST /api/v1/preview/compose
POST /api/v1/preview/artifacts
POST /api/v1/builds
GET  /api/v1/builds/{id}
DELETE /api/v1/builds/{id}
GET  /api/v1/builds/{id}/events
GET  /api/v1/manifest/events
GET  /api/v1/inputs
GET  /api/v1/discover
POST /api/v1/import/propose
```

Требования:

- OpenAPI 3.1 — source of truth для transport contract.
- TypeScript client генерируется в `web/src/api/generated/` и проверяется на drift.
- Bind по умолчанию только `127.0.0.1`/`::1`.
- Initial HTML устанавливает random HttpOnly `SameSite=Strict` session cookie и содержит отдельный CSRF token в nonce-protected meta element; mutations требуют cookie + `X-Iacpack-CSRF`. Tokens отсутствуют в URL/logs.
- Same-origin UI, CORS disabled.
- Workspace root фиксируется при запуске server.
- Compile policy фиксируется при запуске `serve --policy <path>` и не выбирается request-ом.
- API не принимает arbitrary host path для чтения/записи.
- Manifest PUT создаёт missing configured file только с `If-None-Match: *`; обычное изменение требует `If-Match`.
- Save As принимает только WorkspacePath, отказывается overwrite и остаётся внутри configured workspace.
- Request body limit 4 MiB; одновременно максимум 2 build jobs; SSE buffer максимум 1 000 events на job/session с documented resync при overflow.
- Browser никогда не получает secret values.
- Deploy/apply endpoints отсутствуют в v1.1.
- API errors используют RFC 9457 Problem Details с diagnostics extension.
- Manifest event schema версионирован и различает `changed`, `invalid`, `deleted`, `permissionDenied`, `resyncRequired`; каждое событие содержит revision, но не file content.

## 18. Security requirements

### 18.1. Trust boundaries

1. Workspace и manifest считаются недоверенными.
2. Bundle считается недоверенным до полной verification.
3. Подписанный bundle подтверждает автора, но не делает images безопасными.
4. Docker socket является root-equivalent capability.
5. Container не является sandbox для hostile image.
6. Secrets принадлежат оператору и не входят в authoring/build артефакты.

### 18.2. Build safety

- No network, subprocess, Docker, templates with code execution или project hooks.
- Archive/resource paths проверяются до extraction/copy.
- Quotas проверяются до materialization: entry count, individual size, total expanded size и compression ratio.
- Temporary output создаётся с безопасными permissions.
- Files используют atomic same-filesystem replacement; directory build требует новый non-existing target.
- Source workspace не изменяется.

### 18.3. Default deployment policy запрещает

- privileged containers;
- host PID/IPC/network;
- device mappings;
- Docker socket mount;
- external bind mounts;
- added Linux capabilities;
- mutable image tags в release mode;
- unapproved registries;
- host shell hooks;
- secret values в environment;
- arbitrary Compose extensions.

Compile policy и operator policy работают по §9.15: compile policy объявляет только поддерживаемые requirements, operator policy может их только ограничить. Dangerous capability, отсутствующая в v1 schema, не может быть включена policy exception.

### 18.4. Supply chain

- Release artifacts имеют SHA-256 checksums.
- Binaries и bundles поддерживают Sigstore/cosign signatures.
- Release содержит SPDX или CycloneDX SBOM.
- Dependencies и licenses сканируются в CI.
- Critical/high vulnerabilities блокируют release, если нет документированного временного exception с owner и expiry.
- CI actions pin-ятся по full commit SHA.
- Build provenance формируется по SLSA-compatible формату.
- Signature verification по умолчанию offline: explicit detached signature, certificate/proof bundle, pinned local Sigstore trusted-root snapshot и local identity policy. `verify`, `plan` и `apply` не обращаются к transparency service. Обновление trust root — отдельная network-enabled administrative operation вне build/apply.
- Unsigned artifact принимается только если operator policy явно разрешает; release policy требует signature. Проверяются certificate validity at signing time, Rekor inclusion proof и identity constraints. Общая online revocation guarantee не обещается.

### 18.5. Privacy

- Telemetry отсутствует по умолчанию.
- Продукт не отправляет manifest, paths, diagnostics или usage data.
- Любая будущая telemetry — только opt-in после отдельного ADR и privacy notice.

## 19. Нефункциональные требования

### 19.1. Надёжность

- Compiler functions должны быть deterministic и side-effect free.
- File writes используют staging + fsync where supported + atomic rename; directory outputs создаются только в новом target.
- Cancellation очищает только temporary resources.
- Journal выдерживает process crash между любыми actions.
- Unsupported behavior всегда завершается explicit error.

### 19.2. Производительность

На dedicated reference runner (4 pinned vCPU, 8 GiB RAM, local SSD, idle host, release binary, warm filesystem cache):

- validate 200 services и 10 000 declared files: p95 ≤ 2 s, peak RSS ≤ 256 MiB;
- compile/plan без hashing payload: p95 ≤ 2 s;
- UI validation response для 100 services: p95 ≤ 500 ms;
- build overhead сверх чтения/хеширования/записи payload: ≤ 20%;
- API остаётся responsive во время build через bounded worker queue.

Каждый benchmark выполняет 5 warm-up и 30 measured iterations; p95 вычисляется по wall-clock samples, memory — peak RSS. Fixtures: exactly 200 services, 500 graph edges и 10 000 files по 1 KiB. Build overhead сравнивается с отдельным baseline tool, который последовательно read+SHA-256+write те же bytes на том же filesystem.

### 19.3. Совместимость

- Manifest version имеет documented support/deprecation policy.
- Stable v1 schemas поддерживаются минимум две minor product releases после появления replacement.
- Bundle reader обязан либо поддержать format version, либо завершиться `BUNDLE.UNSUPPORTED_VERSION`.
- Provider additions не меняют Docker output golden fixtures.

### 19.4. Accessibility и UX

- Web UI соответствует WCAG 2.2 AA.
- Все операции доступны с клавиатуры.
- Ошибки связаны с соответствующими form controls.
- Color не является единственным носителем смысла.
- Long-running operation показывает progress, cancellation и final result.

### 19.5. Documentation

Репозиторий содержит:

- Quickstart;
- manifest reference;
- CLI reference;
- security/threat model;
- reproducibility guide;
- deployment/recovery guide;
- provider support matrix;
- migration/versioning guide;
- examples;
- ADR index;
- generated artifact format reference.

## 20. Testing и quality gates

### 20.1. Обязательные test levels

- Unit tests для domain/application logic.
- JSON Schema positive/negative corpus.
- Golden tests для canonical manifest, diagnostics, graph, Compose, README и bundle metadata.
- Property tests для ordering, path confinement и semantic round-trip.
- Go fuzz tests для YAML loader, paths/globs, archive parser и source mapping.
- Reproducibility tests при разных cwd, locale, timezone, umask, UID и file creation order.
- Security corpus: traversal, Unicode/case collisions, duplicate entries, archive bomb, symlink escape, YAML aliases, malicious argv и redaction canaries.
- Integration tests Docker Compose renderer/provider.
- Fault injection до/после каждого journal transition.
- E2E: apply, health failure, task failure, interruption, resume, update и retained volumes.
- API contract tests.
- UI unit/component tests и Playwright user journeys.

### 20.2. Quality commands

Минимальный gate:

```text
go fmt / gofmt check
go vet ./...
staticcheck ./...
go test -race ./...
govulncheck ./...
pnpm lint
pnpm typecheck
pnpm test
pnpm build
pnpm playwright test
```

Bootstrap фиксирует named targets: `check`, `test-race`, `test-security`, `test-integration`, `test-e2e`, `test-ui`, `test-release`, `test-all`. Local agent обязан запускать доступные deterministic targets; Docker/version/OS matrix выполняется CI; signing identity, dedicated performance runner и final publish являются human-controlled release gates. Нельзя объявлять недоступный external gate пройденным.

### 20.3. Coverage

- Core domain/application/input/bundle/runtime: ≥ 90% line coverage.
- Для security-critical path/archive/journal/cleanup ведётся explicit decision/scenario matrix с тестом каждого outcome; стандартный Go toolchain не используется для недостоверной «branch coverage» метрики.
- Coverage не заменяет scenario, fuzz и mutation-oriented tests.
- Нельзя снижать threshold или без объяснения обновлять golden files.

## 21. Observability

- Human logs и machine diagnostics разделены.
- Log records имеют timestamp только в runtime logs, level, run ID, phase, action ID и code.
- Build artifacts не получают runtime timestamps.
- Secret redaction применяется до persistence.
- `status --output json` показывает active bundle, last run, health summary, pending recovery и owned resources без credentials.
- Неуспешный apply всегда сообщает путь к private log и безопасные next steps.

## 22. Release и distribution

- SemVer для продукта.
- Reproducible binaries для Linux amd64/arm64; CLI binaries также для macOS/Windows.
- Release создаётся только из signed tag через least-privilege CI job.
- Артефакты: binaries, checksums, signatures, SBOM, provenance, schema archive и changelog.
- Installation docs не предлагают `curl | sh`.
- Upgrade state/schema выполняется explicit migrator-ом с backup и rollback только самого metadata migration.
- Public release запрещён при failing required gate.

## 23. Acceptance criteria v1.0

Релиз v1.0 принят только если одновременно выполнено:

1. YAML и эквивалентный JSON дают одинаковые canonical model и artifact hash.
2. Duplicate keys, unknown fields, aliases, unsupported versions и malformed values дают stable diagnostics.
3. Path traversal, absolute workspace paths, escaping links и path collisions отклоняются.
4. Два build на поддерживаемых host OS дают byte-identical bundle при одинаковых inputs/tool version.
5. Изменение declared input меняет digest; undeclared file не влияет на output.
6. `validate/build` не используют сеть, не запускают subprocess/containers и не меняют workspace.
7. Secret canary не встречается в bundle, plan, Compose, docs, logs, diagnostics или process args.
8. Bundle не содержит undeclared files или unsafe archive entries.
9. Release images pinned by digest; drift lockfile приводит к failure.
10. Generated Compose проходит `docker compose config --quiet` на поддерживаемой matrix.
11. Cycle diagnostic показывает actual cycle; graph ordering deterministic.
12. Reference two-service fixture успешно устанавливается и достигает health.
13. Повторный apply не создаёт duplicate resources.
14. Failure на каждой apply stage даёт корректный exit code, journal и recovery action.
15. Interrupted action корректно определяется как completed/pending/unknown при resume.
16. Automatic cleanup не удаляет pre-existing resources или volumes.
17. Destroy сохраняет volumes без explicit destructive flags.
18. CLI JSON соответствует versioned schema; stdout/stderr/color contracts соблюдены.
19. Performance limits раздела 19.2 выполняются.
20. Core coverage и все security/fuzz/race gates выполняются.
21. Release содержит checksums, signatures, SBOM и provenance.
22. Thin installer не содержит orchestration logic и проходит ShellCheck.
23. Offline bundle устанавливается без network access.
24. Documentation позволяет новому пользователю пройти init → build → apply без скрытых шагов.
25. Ни один blocker/critical security finding не остаётся открытым.

## 24. Acceptance criteria v1.1 Web UI

1. Пользователь создаёт two-service manifest без ручного YAML.
2. CLI и API возвращают эквивалентные diagnostics и byte-identical preview.
3. Import → visual edit → export сохраняет deployment semantics.
4. Изменение только node positions не меняет infrastructure output.
5. External edit обнаруживается без reload страницы.
6. Stale save получает conflict и не перезаписывает новый файл.
7. Invalid external YAML не уничтожает last valid visual state.
8. UI не читает файлы вне configured workspace.
9. Browser не получает secret values.
10. WCAG 2.2 AA и keyboard flows подтверждены automated/manual checks.

## 25. Обязательные ADR до соответствующей реализации

1. Go single-binary architecture.
2. JSON Schema как нормативный manifest contract.
3. Pure compiler и explicit inputs.
4. Typed path domains и запрет arbitrary rewriting.
5. Secret references и file delivery.
6. Canonical lockfile/deterministic bundle.
7. Docker Compose v1 provider profile.
8. Journaled apply и compensating cleanup.
9. Container tasks и запрет host hooks.
10. Signing/provenance model.
11. Loopback API, workspace confinement и ETag saves.
12. UI semantic round-trip и `x-ui`.
13. Offline OCI image layout.
14. Provider extension/conformance process.
15. Compile/operator policy layering и immutable default policy.

## 26. Definition of Done для любой функции

Функция считается завершённой, только если:

- поведение соответствует schema/spec/ADR;
- success, boundary и failure paths протестированы;
- diagnostics и exit behavior задокументированы;
- security, secret, path и determinism invariants не нарушены;
- CLI/API/UI используют общий application path;
- docs/examples обновлены;
- format/lint/type/race/unit/integration gates пройдены;
- нет placeholder, disabled test или silent fallback;
- handoff перечисляет изменённые contracts и реально запущенные проверки.

