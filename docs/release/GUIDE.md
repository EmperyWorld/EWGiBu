# Руководство оператора

Как из Compose-проекта получить манифест, архив и запущенный стек без ручной
дописи `user`, `environmentFrom` и второй базы. Контракт продукта — `TZ.md`.
Нормативные детали: [manifest-v1alpha1](../reference/manifest-v1alpha1.md),
[runtime-apply-v1](../reference/runtime-apply-v1.md),
[compose-import-fidelity](../reference/compose-import-fidelity.md).
Команды по одной строке: [CLI.md](CLI.md). Короткий прогон: [CONSOLE.md](CONSOLE.md).

Секретные значения в манифест, lock, bundle, план, лог и аргументы процесса
не попадают. В команды передаётся только путь к файлу секрета.

## 1. Сборка

Из корня репозитория, Go 1.27.1:

```bash
make build
./bin/iacpack version
```

Дальше `iacpack` — это `./bin/iacpack` или тот же бинарник в `PATH`.
`validate`, `inspect`, `plan` и `verify` не запускают код проекта и не ходят
в сеть. `build` запускает менеджер пакетов только для `spec.builds`.

## 2. Импорт Compose

Propose ничего не пишет в workspace. Черновик остаётся в памяти UI или в JSON,
который вы сохраняете сами.

```bash
iacpack import propose \
  --workspace /path/to/project \
  --compose compose.yaml \
  --dockerfile app/Dockerfile \
  --profile worker \
  --name shop \
  --output json
```

Повтор `--compose`, `--dockerfile` и `--profile` добавляет источники. Несколько
имён в одном флаге можно перечислить через запятую. `--import-strict` поднимает
policy-отказы до error.

В UI: **Import draft…** → Propose. Повторное нажатие Propose отправляет текущий
черновик, поэтому уже заданный `user` сохраняется. Первый Propose, когда
черновика ещё нет, собирает проект заново. На шаге проверки видны сервисы, у
которых импорт выставил `user` или `environmentFrom`.

### 2.1. `service.user`

Порядок такой:

1. `user`, уже стоящий в черновике (`--existing-project-file` в CLI, поле
   `existingProject` в API, повторный Propose в UI), не перезаписывается.
2. `user:` из Compose остаётся как есть, включая `0` и `0:0`.
3. Если Compose `user` не задал, propose читает локальный `Config.User` образа
   (`podman image inspect`, затем `docker`, таймаут 3 секунды, без pull).
   Число (`1000`, `0`, `0:0`, `1000:1001`) записывается в `service.user`.
   Имя (`keycloak`, `nginx`) не записывается: диагностика `IMPORT.USER_NAME`
   называет сервис и строку образа.
4. Если образ не локальный или у него пустой `USER`, берётся последняя
   инструкция `USER` в Dockerfile сервиса. `${APP_UID:-1000}` раскрывается в
   значение по умолчанию. Число записывается, имя снова даёт `IMPORT.USER_NAME`.
5. Если нет ни образа, ни `USER` в Dockerfile, предупреждение
   `IMPORT.USER_ABSENT`. Подсказка: загрузить образ или дописать `USER`, затем
   импортировать снова.
6. Образ есть, `Config.User` пустой и в Dockerfile нет `USER` — `service.user`
   не ставится и отдельной диагностики нет.

Имя пользователя не годится для `chown` на rootless Podman. Для него нужен
числовой uid.

```bash
iacpack import propose \
  --workspace /path/to/project \
  --compose compose.yaml \
  --existing-project-file imports/import-draft.json \
  --output json
```

### 2.2. `environmentFrom`

Сервисы с одной и той же pull-ссылкой образа группируются вместе. Самый полный
набор переменных (при равенстве — лексикографически меньший id) становится
родителем. У остальных propose ставит `environmentFrom: <родитель>` и оставляет
в `environment` только ключи, которых нет у родителя или чьи значения отличаются.
Одинаковые карты сливаются: у ребёнка `environment` пропадает. Ребёнок, который
уже содержит все значения родителя, не меняется. Цикл и ссылка на себя запрещены.

Диагностика `IMPORT.ENV_FROM` перечисляет, кто наследует и какие ключи
переопределены. После этого `plan` не печатает `PLAN.ENV_MISSING` для этой
пары. Если два сервиса на одном образе и ключ есть только у одного, а
`environmentFrom` нет, `plan` предупреждает `PLAN.ENV_MISSING` и подсказывает
поставить наследование.

Ключи окружения не переименовываются. Хост в значении, совпавший с
`container_name`, заменяется на id сервиса.

### 2.3. Что импорт не делает

- Не читает `.env`, `--env-file` и окружение оболочки. Берётся только
  `${VAR:-default}`, записанный в самом Compose.
- Не создаёт `spec.tasks` и не создаёт `spec.databases`. Дерево
  `**/migrations` с `baseline.sql` и/или `pending/` даёт предупреждение
  `IMPORT.MIGRATE_GAP` и шаблон задачи. Каталог `applied/` — история, его
  не проигрывают.
- Не подставляет секретные значения. Похожие на секрет ключи становятся
  объявлением `{secret: id}`. `token` — отдельный сегмент имени: `tokens` и
  `token-ttl` секретом не являются, `api-token` является.
- `gpu: all` без CDI по-прежнему останавливает apply. `devices:` уровня
  сервиса в черновик не входит.

## 3. Вторая база на уже существующем Postgres

Блок общий. Имя `ewa` ниже — только образец. Второго контейнера Postgres нет.

```yaml
spec:
  databases:
    ewa:
      on: postgres
      name: ewa
      ownerFrom: postgres
      migrations: shared/api/ewa_platform/migrations/applied
      urlParameter: ewa-database-url
```

| Поле | Смысл |
| --- | --- |
| `on` | Сервис сервера. У него должен быть healthcheck, иначе semantic error. |
| `name` | Имя базы: `^[A-Za-z_][A-Za-z0-9_]{0,62}$`. |
| `ownerFrom` | Сервис с `POSTGRES_USER` (литерал или параметр) и паролем: `POSTGRES_PASSWORD` как секрет или любой secret mount. |
| `migrations` | Каталог в workspace. В lock попадают `*.sql` и `**/*.sql`. |
| `urlParameter` | Параметр deploy. Если его не было, он объявляется строкой без default. Compile подставляет DSN, поэтому `plan` не требует `--set` для него. |

Compile, а не правка исходного YAML, добавляет задачу `db-<id>`:

- фаза `migration`, `rerun: oncePerDeployment`;
- образ и сети сервиса `on`;
- `dependsOn`: `on` в состоянии `healthy`;
- entrypoint `["/bin/sh"]`, command `["/iacpack/db/migrate.sh"]`;
- mount только для чтения: payload в `/iacpack/db`;
- секрет пароля на `/run/secrets/db`;
- `user` — числовой `user` сервиса `on`, иначе `"0"`, чтобы задача могла прочитать файл пароля.

Скрипт создаёт базу, если её нет, и таблицу `iacpack_schema_migrations`
(`name`, `hash`). Файлы идут по имени. SHA-256 файла сравнивается с `hash`.
Совпадение — файл пропускается. Отличие — файл выполняется снова и хеш
обновляется (`INSERT … ON CONFLICT DO UPDATE`). Старая таблица без колонки
`hash` получает `ALTER TABLE … ADD COLUMN IF NOT EXISTS`.

Повторный запуск изменённого файла должен быть безопасен (`CREATE TABLE IF NOT
EXISTS`, `INSERT … ON CONFLICT`). Новый файл с новым именем — обычный
инкремент. Байт пароля в скрипт, lock и bundle не записывается.

DSN в окружении сервиса, где стоял `{parameter: ewa-database-url}`:

```text
postgresql://<user>:<secret>@postgres:5432/ewa
```

Пользователь — тот же, что у `ownerFrom`. Пароль — ссылка на секрет, не значение.
Порт в этой сборке — 5432, хост — id сервиса `on`.

После правки SQL:

```bash
iacpack lock --file iacpack.yaml
iacpack build --file iacpack.yaml --format bundle --output db.iacbundle
iacpack apply db.iacbundle --secret-file db-password=/path/to/db-password --yes
```

Меняется хеш только задачи `db-<id>`. Другие `oncePerDeployment` с тем же
входом не запускаются.

## 4. Lock и устаревший черновик

```bash
iacpack lock --file iacpack.yaml --policy compile-policy.json
iacpack validate --file iacpack.yaml --locked
iacpack build --file iacpack.yaml --format bundle --output project.iacbundle
```

`release` требует `repository@sha256:<64-hex>` у каждого образа. Тег в
`development` даёт warning `LOCK.IMAGE_UNRESOLVED` и lock при этом завершается.
Реестр `localhost` в политике по умолчанию запрещён (`POLICY.REGISTRY_DENIED`);
для локальных тегов нужен compile policy, где он разрешён.

`plan` и `apply` сравнивают черновик workspace с lock внутри bundle, если задан
`--workspace` или `./iacpack.yaml` называется тем же проектом. Другое имя
проекта в текущем каталоге само по себе не считается. Расхождение — код
`LOCK.DRIFT`, сообщение `lock устарел`, код выхода 6, контейнеры не
создаются. Подсказка: `iacpack diff --from <манифест> --to <lock или bundle>`.

## 5. Секреты, владелец и SELinux

```bash
iacpack apply project.iacbundle \
  --values values.json \
  --secrets-dir ./secrets \
  --instance default \
  --yes
```

`--secrets-dir` берёт каждый обычный файл: имя файла — id секрета. Содержимое в диагностику и в аргументы не попадает. `--secret-file id=/path` заменяет тот же id из каталога.

В разделе «Секреты» у каждого объявления есть список «Сгенерировать как»: пароль, порт или имя. «Распознать тип» смотрит на id, описание и ключ окружения, где секрет подставлен. «Сгенерировать» пишет `secrets/<id>` в каталоге workspace: каталог `0700`, файл `0600`. Ответ сервера содержит только id и путь. Значение не показывается и в манифест не записывается. Если файл уже есть, замена спрашивается отдельно. Дальше тот же каталог передаётся как `--secrets-dir ./secrets`.

До записи состояния и до контейнеров apply печатает один отчёт. `APPLY.IMAGE_PULL` перечисляет теги реестра, которых ещё нет. Подсказка — команда `podman pull` с именами через пробел, без запятых. `APPLY.IMAGE_LOCAL` останавливает запуск, если локального тега нет: `pull` его не найдёт. `APPLY.HOST_PATH` — абсолютный путь из values не существует. `SECRET.MISSING` — нет файла секрета. `PARAM.MISSING` — у параметра deploy нет ни default, ни значения в `--values` / `--set`. Ошибка здесь — код 6, каталог состояния не создаётся.

Файл копируется в каталог секретов экземпляра: каталог `0700`, файл `0440`.
Шире `0440` режим не ставится. В Compose секрет монтируется как
`../secrets/<id>` относительно `rendered/compose.yaml`. У podman-compose у этого
bind нет метки `:z`.

На SELinux enforcing (`/sys/fs/selinux/enforce` = 1) каталог секретов
помечается `container_file_t` после записи и ещё раз после `chown`.

Перед `compose up` и `compose run` числовой `user`, кроме `0` и `0:0`,
получает `podman unshare chown` и `chmod 0440`. Имя пользователя, пустой
`user` и uid 0 этот шаг пропускают. `USER` образа `0` или `0:0` ошибкой не
считается. В rootless uid 0 внутри контейнера — это пользователь хоста, он
читает файл `0440`, принадлежащий ему. Чужой uid (`1000`, `999`) читает файл
только после `chown`.

Тот же `chown` делается для записываемых bind и точек монтирования томов
сервиса с числовым ненулевым `user`. Если `user` пустой и у образа нет
`Config.User` (так у официального Postgres), entrypoint потом переходит на
uid 999. Каталог payload в этом случае получает режим `0777`, чтобы процесс
создал файл лога. Каталог на хосте, например каталог моделей, так не
открывается. Имя тома у движка — `<slug>_<id>`, не логический id.

Host bind, в том числе только для чтения, помечается `container_file_t` и в
Compose получает `selinux: z`. Без метки SELinux отвечает `Permission denied`
на файл, который пользователь хоста читает.

Повторный apply того же архива печатает `noop: active bundle already matches`
и контейнеры не пересоздаёт, когда совпали digest архива, semantic hash, хеши
`oncePerDeployment` и штамп хоста (`render-host.txt`: узлы GPU и правило
devices-only). Смена штампа, заполненного host path или хеша задачи снова
запускает apply. Задача с `oncePerDeployment` не запускается, если хеш её
command, entrypoint, user и байтов файлов на монтировании совпал. Байты
секрета в хеш не входят.

Задача не получает переменную `IACPACK_SECRET_*`. Пароль она читает из
смонтированного файла. В логе, `explain`, `task-inputs.json` и аргументах
процесса байтов секрета нет.

## 6. GPU

`gpu: all` требует CDI `nvidia.com/gpu=all`. Нет CDI — `APPLY.GPU_UNAVAILABLE`,
код 6, `compose up` не начинается.

`gpu: devices-only` монтирует узлы `/dev/nvidia*` (`nvidiactl`, `nvidia-uvm`,
`nvidia-uvm-tools`, `nvidia-modeset`, `nvidia` и цифры, а также файлы устройств
в `/dev/nvidia-caps`) и выставляет `LD_LIBRARY_PATH` из `ldLibraryPath`.
`NVIDIA_VISIBLE_DEVICES` становится списком индексов этих карт (`nvidia0` →
`0`). `void` прячет уже смонтированную карту от CUDA, и llama пишет
`no CUDA-capable device`. На хосте с включённым SELinux этот контейнер
получает `security_opt: [label=disable]`: тип `container_t` не открывает
`/dev/nvidia*` (`xserver_misc_device_t`), и CUDA пишет то же сообщение, хотя
`nvidia-smi` на хосте карту видит. Конфайнмент снимается только у этого
сервиса. Уже собранный архив для этого пересобирать не нужно: apply рендерит
Compose заново. Повторный запуск того же архива не будет `noop`, пока штамп
хоста не совпадёт с новым правилом.

`iacpack explain <bundle> <service>` печатает образ, ref, `user`, gpu,
`ldLibraryPath`, устройства, ключи окружения и `environmentFrom`. Значений
секретов там нет.

Если `service.ready` не дождался, к `APPLY.ACTION` добавляются последние 80
строк лога этого сервиса.

## 7. Переносимый комплект

На машине, где проект уже залочен:

```bash
iacpack bundle \
  --workspace /path/to/project \
  --output-dir /path/to/kit \
  --policy compile-policy.json \
  --embed-local-images
```

Каталог получает:

| Файл | Содержимое |
| --- | --- |
| `iacpack.lock.json` | Копия lock workspace. |
| `values.json` | Default параметров deploy. Имена, похожие на секрет, не пишутся. |
| `secrets.txt` | Id обязательных секретов, по одному на строку. Значений нет. |
| `images.txt` | `ref`, затем таб, затем `pull`, `local-only` или `load`. Строка `load` ещё содержит относительный путь к tar. |
| `host-paths.json` | Шаблон. У каждой строки `value` пустой. Литерал хоста получает свой параметр `host-<сервис>-<цель>` и поле `literal` как подсказку. |
| `project.iacbundle` | Архив, собранный с той же `--policy`, что и lock. |
| `images/*.tar` | Только при `--embed-local-images`: `podman save` или `docker save` каждого `local-only`. Строка в `images.txt` становится `load`. |

Пустой шаблон путей и образ, оставшийся `local-only`, — предупреждения
`PORTABLE.HOST_PATH` и `PORTABLE.LOCAL_IMAGE`. Файлы и архив при этом
записываются, команда завершается с кодом 0. Неудачный `save` — ошибка.

На другой машине `plan` и `apply` каталога — жёсткая проверка до контейнеров:

- одна `PORTABLE.HOST_PATH` перечисляет все пустые и неабсолютные `value`;
- одна `PORTABLE.LOCAL_IMAGE` перечисляет все `local-only` и `load`, которых
  нет в локальном движке (`image inspect`, без pull).

Код выхода 6, состояние не меняется.

Образы в `.iacbundle` не лежат. `apply` делает `image inspect` и, если образа нет, `pull`. Тег из реестра так скачивается на новой машине. Локальный тег (`localhost/…` или имя без реестра) `pull` не достанет. Его можно привезти двумя способами: `podman load` архива `*.tar` или собрать тем же тегом из скопированного репозитория. Оба способа остаются.

Несколько локальных образов в одном tar собираются так:

```bash
podman save -m -o images.tar image-a:tag image-b:tag
```

Без `-m` (`--multi-image-archive`) остальные имена становятся тегами первого
образа. `podman image list` тогда показывает один и тот же IMAGE ID у всех
строк, и контейнер фронтенда или tesseract запускает чужой процесс.
`--embed-local-images` пишет каждый образ своим файлом, флаг `-m` там не нужен.

```bash
scripts/prepare-apply-host.sh /path/to/kit --secrets /path/to/secrets --iacpack ./iacpack
```

Скрипт ставит Podman и `podman-compose`, загружает каждый `images/*.tar` и проверяет бинарник, файлы секретов, путь к моделям и `/dev/nvidia0`. Контейнеры он не запускает. В конце печатает `apply` с `--secrets-dir`.

В `host-paths.json` у каждой строки заполнить абсолютный `value`, например `/srv/models`. Относительный путь проверка комплекта не принимает.

```bash
iacpack plan /path/to/kit --values values.json --secrets-dir /path/to/secrets
iacpack apply /path/to/kit \
  --values values.json \
  --secrets-dir /path/to/secrets \
  --instance default \
  --yes
```

Проверки комплекта читают каталог. Затем берётся `project.iacbundle` (если архивов несколько — первый по имени). Заполненное `value` подменяет host bind этого сервиса или задачи, в том числе путь, который в манифесте был литералом. `--set` того же параметра, переданный в команду, остаётся как задал оператор.

Повторный apply того же архива и тех же заполненных путей — noop, пока не
изменился штамп хоста (узлы GPU и правило devices-only). Смена `value`
применяется снова. В `secrets.txt` только имена. Файлы секретов лежат отдельно
и передаются через `--secrets-dir`.

Каталог без `project.iacbundle`, у которого проверки прошли, отвечает текстом
`portable check passed` и контейнеры не запускает. Обычный `iacpack bundle`
архив записывает, поэтому этот ответ бывает только у каталога, собранного
вручную без архива.

## 8. Apply, задачи, коды выхода

Движок: Podman, если он есть, иначе Docker. Явно:
`--container-engine podman|docker` или `IACPACK_CONTAINER_ENGINE`.
Сокет: `--docker-socket` или `IACPACK_DOCKER_SOCKET`.

Каждый сервис — `compose up -d --no-deps --force-recreate <service>`.
Готовность — `ps`/`inspect` по меткам compose, не `compose ps --status`.
Сбой одного сервиса не отменяет остальные. Задачи идут как
`compose --profile iacpack-task run -T --rm --no-deps <task>` и в обычный
`compose up` не попадают. `-T` убирает TTY: иначе `psql` ждёт пароль и apply
стоит на этой задаче. Вывод задачи печатается по ходу. Лимит — `timeout` из
манифеста, не пять минут.

Если у сервиса задан `entrypoint`, apply перед ним выполняет `cd` в
`Config.WorkingDir` образа и пишет тот же путь как `working_dir`. Иначе
Podman стартует процесс из `/`, и `uvicorn server:app` не находит модуль.

После того как сервис Postgres стал healthy, отсутствующая база `POSTGRES_DB`
создаётся внутри образа `postgres`. Контейнер, у которого те же переменные
только в окружении клиента, для этого не используется: в нём нет `psql`.
Второе имя базы, которого нет в `spec.databases`, apply не создаёт.

Сеть с `external: true` Compose не создаёт. Apply делает `network create` для
её имени, если сети ещё нет. Обычную сеть по-прежнему создаёт Compose.

`rerun`:

| Значение | Поведение |
| --- | --- |
| `always` | Каждый apply. |
| `oncePerDeployment` | Пропуск, если хеш command, entrypoint, user и байтов файлов mount совпал с `task-inputs.json`. Новый bundle сам по себе задачу не перезапускает. |
| `manual` | Только `iacpack task run <project> <task>`. |

`<project>` в `status`, `destroy`, `resume` и `task run` — это `metadata.name`,
не путь к архиву. Тома при `destroy` сохраняются. Удаление — отдельный
`--delete-volumes` и `--yes`.

| Код | Когда |
| --- | --- |
| 0 | Успех, в том числе noop и `bundle` с предупреждениями. |
| 2 | Ошибка вызова. |
| 3 | Манифест или проверка не прошли. |
| 4 | Ошибка ввода-вывода. |
| 5 | Архив повреждён или не читается. |
| 6 | Остановка до изменений: `APPLY.IMAGE_LOCAL`, `APPLY.HOST_PATH`, `SECRET.MISSING`, `PARAM.MISSING`, GPU, устаревший lock, пустой комплект. |
| 7 | Часть изменений уже произошла. Повторный apply продолжает с журнала, отката нет. |
| 70 | Внутренняя ошибка. |

`--output json` у каждой команды сверяется со схемой в `schemas/cli/v1`.

## 9. Границы

- Манифест конкретного приложения это руководство не переписывает. Числовой
  `user` и `environmentFrom` появляются при импорте, если образ локальный или
  в Dockerfile есть `USER`.
- `gpu: all` без CDI останавливает apply. `devices-only` монтирует
  `/dev/nvidia*`, ставит `NVIDIA_VISIBLE_DEVICES` в индекс карты и на
  SELinux enforcing снимает конфайнмент только у этого сервиса
  (`label=disable`).
- Несколько локальных образов в одном tar требуют `podman save -m`.
- Повторный apply того же архива — `noop`, пока не изменились хеш задачи,
  заполненный host path или штамп GPU на хосте.
- Слово `tokens` и сегмент `token-ttl` секретом не считаются.
- Изменённый SQL-файл исполняется снова и должен выдержать второй запуск.
- UI не вызывает apply. Запуск стека остаётся командой в терминале.
