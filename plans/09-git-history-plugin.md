# План 9. Git-история как встроенный подключаемый плагин

Статус: проект.
Приоритет: средний; выполнить до Plan 8, этапа 2, если sync должен записывать Git-ревизии.
Зависимости: Plan 1 и Plan 2 готовы. Основание — существующие `packages/history`,
`core.History` и настройки плагинов в `users.yaml`.

## 1. Цель

Сделать Git-историю необязательной встроенной возможностью, которой администратор
управляет из Settings → Plugins без редактирования `config.yaml` и рестарта.
При выключенной истории заметки продолжают создаваться, редактироваться,
удаляться и синхронизироваться; недоступны только ревизии, diff, restore и
Git-backed trash.

Результат для пользователя:

1. В списке плагинов есть «Git history» с описанием и состоянием.
2. Admin включает/выключает его в веб-интерфейсе; состояние применяется сразу.
3. Пока плагин выключен, кнопка History, API истории и Git-коммиты недоступны.
4. При повторном включении плагин использует существующий `.git` в vault или
   инициализирует его только в режиме `managed`.

## 2. Почему нельзя сделать это одной регистрацией

Сейчас `apps/server/main.go` открывает `packages/history` напрямую и вызывает
`notes.AttachHistory`. Маршруты `/api/history/*` и `/api/diff/*` находятся в
`packages/api`, а `HistoryPanel` — постоянная часть React UI. Текущий
`plugins.Manager` проверяет enabled только при обращении к plugin routes;
`InitAll` вызывается при старте даже для выключенного backend-плагина.

Поэтому перенос одного `history.Open` в `builtin.Plugin` был бы ложным
выключением: Git всё ещё мог бы писать коммиты. Нужны динамический lifecycle
и отдельный capability contract для UI/API.

## 3. Границы

### Входит

- Git backend как built-in plugin `git-history` (compiled into binary);
- включение/выключение и настройки через существующий admin Plugins UI;
- plugin-owned REST endpoints и HistoryPanel, показываемая только при enabled;
- безопасное attach/detach history backend к `NoteService`;
- совместимость существующего `history:` config как bootstrap default;
- существующие Git ревизии, restore, diff и trash без изменения формата.

### Не входит

- загрузка Go-плагинов из файлов, изолированные процессы или marketplace;
- remote Git push/pull и UI для репозиториев;
- замена Git на новый history store;
- синхронизация заметок из Plan 8 и CRDT.

## 4. Дизайн

### 4.1. Плагин и настройки

Новый built-in plugin имеет manifest:

```go
ID:          "git-history"
Name:        "Git history"
Version:     "1.0.0"
Description: "Revisions, diff, restore and Git-backed trash for the vault."
Settings: []pluginsdk.SettingSpec{
  {Key: "mode", Label: "Mode", Default: "managed"},
  {Key: "externalDebounceSec", Label: "External edit debounce (seconds)", Default: "3"},
  {Key: "squashWindow", Label: "Autosave squash window", Default: "5m"},
}
```

`history.enabled`, `history.mode` и `history.externalDebounceSec` из
`config.yaml` становятся только начальными значениями для первой загрузки.
После появления записи `plugins.git-history` в `users.yaml` она имеет
приоритет. Документировать миграцию, но не переписывать `config.yaml` из UI.

Для `mode: external` плагин требует существующий Git-репозиторий. Для
`managed` он может создать репозиторий. Неверная настройка оставляет плагин
disabled и показывает admin понятную ошибку, не ломая NoteService.

### 4.2. Жизненный цикл

Расширить plugin runtime, не вводя goroutine в core:

```go
// необязательный интерфейс для backend plugins
type Toggleable interface {
    Enable(host Host) error
    Disable() error
}
```

- `Init` регистрирует маршруты и лёгкие неизменяемые ресурсы.
- Manager вызывает `Enable` только для включённого плагина при запуске.
- После `PUT /api/admin/plugins/git-history` Manager сериализованно вызывает
  `Enable` или `Disable`; сначала сохраняется валидная настройка, затем
  lifecycle, при ошибке состояние откатывается и UI получает ошибку.
- `Disable` обязан остановить timers/subscriptions и снять capability до
  возврата. Это предотвращает новый Git commit после ответа admin API.
- `CloseAll` сначала disables active plugins, затем вызывает `Close`.

Не использовать `pluginEnabled` как единственную проверку: это transport guard,
а не остановка фоновой работы.

### 4.3. Capability истории в core

`NoteService.AttachHistory` заменить на потокобезопасное явное управление:

```go
func (s *NoteService) SetHistory(h History, externalDebounce time.Duration)
func (s *NoteService) DetachHistory(h History)
```

`DetachHistory` снимает backend только если передан тот же instance, отменяет
external debounce timers и ждёт завершения уже начатого `Record` под
соответствующим path lock. Методы `RestoreNote`, `Deleted` и `PurgeDeleted`
остаются в core, но при `nil` возвращают typed `ErrHistoryDisabled`.

Git plugin подписывается на события filesystem только если это необходимо для
external mode; собственные core mutations по-прежнему вызывают `record`. Перед
переносом надо исследовать точный event flow, чтобы не получить два коммита на
одну запись. Если plugin subscription не позволяет соблюсти этот инвариант,
допустим минимальный port `HistoryRecorder` в core, но реализация Git остаётся
в plugin.

### 4.4. API и permissions

Удалить жёстко заданные history/diff routes из `packages/api/router.go`.
Плагин регистрирует маршруты под своим namespace:

| Метод | Путь |
| --- | --- |
| GET | `/api/plugins/git-history/history/*path` |
| GET | `/api/plugins/git-history/diff/*path` |
| POST | `/api/plugins/git-history/restore/*path` |
| GET/POST | `/api/plugins/git-history/trash` / `/trash/restore` |

Плагинному маршруту требуется отдельная permission-проверка `history:read`
(и `notes:edit` для restore). Нынешний SDK передаёт `net/http` handler без
identity/permission helper, поэтому расширить SDK минимально:
`Host.Routes().Handle(method, path, Permission, handler)` либо middleware
`RequirePermission`. Изменение SDK — breaking: повысить `APIVersion` до 2.0.0
и обновить bundled plugins в том же PR. Не передавать Gin Context в SDK.

При выключенном плагине routes отвечают 404. Старые `/api/history/*` и
`/api/diff/*` поддерживать один минорный релиз как permission-protected `307`
на новый URL либо объявить breaking change следующего major release — решение
в ADR. Не держать два независимых handler-а.

### 4.5. Web UI

Новый frontend extension point не нужен для одной панели. `HistoryPanel` и
кнопка в `NotePage` остаются в web bundle, но используют plugin API URLs и
показываются только если статус `git-history.enabled === true` и есть
`history:read`. После `plugin.changed` закрывать открытую панель и
инвалидировать queries.

`GET /api/settings` перестаёт экспортировать общий `history` флаг. Клиент
берёт capability из `GET /api/plugins`; предупреждение при удалении зависит от
статуса `git-history`, не от `settings.history`.

## 5. Этапы

| Этап | Результат | Статус |
| --- | --- | --- |
| 0 | ADR: lifecycle, config migration, SDK v2 и стратегия legacy routes | не начат |
| 1 | Toggleable lifecycle в Manager, serialisation/rollback, тесты | не начат |
| 2 | потокобезопасный Set/Detach History в core, typed errors, тесты гонок | не начат |
| 3 | Git-history builtin plugin, settings/default migration, event flow | не начат |
| 4 | plugin routes + SDK permission contract, удаление core routes | не начат |
| 5 | переключение UI, статусы, i18n en/ru, help/API/docs | не начат |
| 6 | migration e2e, race/crash tests и release notes | не начат |

Каждый этап — отдельное задание агенту и review. Этапы 1–2 нельзя сливать с
рефакторингом UI: они определяют гарантию, что «выключено» означает отсутствие
записи Git.

## 6. Риски

- **Гонка disable и SaveNote:** lifecycle lock + `DetachHistory` по instance;
  тест: много concurrent saves, disable, после успеха disable нет новых commits.
- **Поломка корзины:** Git-backed trash недоступен с plugin disabled. Обычное
  удаление остаётся, но UI честно предупреждает о невозможности restore.
- **SDK v2 ломает plugins:** обновить bundled plugins; external plugin должен
  быть пересобран, это явно указать в release notes и `docs/plugin-sdk.md`.
- **Ошибка конфигурации:** не активировать частично созданный Git backend;
  вернуть admin API ошибку и сохранить предыдущее enabled state.
- **Plan 8:** sync может работать без Git; если history включена, sync save
  создаёт максимум одну Git-ревизию на подтверждённую операцию.

## 7. Критерии приёмки

1. Новый инстанс с default config показывает Git history как включённый
   встроенный plugin, а existing `history:` config сохраняет поведение.
2. Admin выключает plugin через UI: следующий save не создаёт commit, все его
   endpoints отвечают 404, кнопка History исчезает без reload.
3. После включения managed mode новые saves снова создают Git-ревизии;
   revision/diff/restore/trash работают с прежними данными.
4. `external` mode не создаёт repo/commit для platform saves и корректно
   показывает внешний Git log согласно прежней семантике.
5. Permission `history:read` по-прежнему закрывает log/diff/trash, а ACL
   исключает скрытые пути из каждого plugin endpoint.
6. Параллельные save/toggle не дают data race (`go test -race` для пакетов
   core/plugins/history) и не записывают commit после завершившегося disable.
7. `go test ./...`, `go vet ./...`, `cd apps/web && npm run build` зелёные;
   обновлены docs/plugin-sdk.md, docs/api.md, docs/architecture.md и оба
   языка i18n.
