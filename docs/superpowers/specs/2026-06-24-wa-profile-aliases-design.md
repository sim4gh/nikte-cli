# Alias para perfiles de WhatsApp en `nk wa`

**Fecha:** 2026-06-24
**Estado:** Diseño aprobado (Codex-revisado), pre-implementación
**Construye sobre:** [2026-06-23-wa-multi-profile-design.md](2026-06-23-wa-multi-profile-design.md)

## Problema

Los perfiles de WhatsApp se seleccionan con `-p N` (1-4). Los números no son
memorables: el usuario no recuerda cuál cuenta es la 2 vs la 3. Queremos poder
nombrarlos (`trabajo`, `personal`) y usar ese nombre en lugar del número.

## Decisiones de diseño

1. **`-p` acepta número o alias.** El número (1-4) sigue funcionando siempre
   como fallback; un alias definido resuelve al número del perfil.
2. **Asignación con comando dedicado** `nk wa alias`, no al vincular.
3. **Salida legible:** los mensajes muestran el alias si existe, si no el número.
4. **Reservados / formato:** un alias debe casar `^[A-Za-z0-9_-]{1,32}$` (así las
   sugerencias de comando quedan shell-safe), no puede ser puramente numérico
   (ambigüedad con 1-4) ni la palabra `all` (la usa `nk wa ls --all`).
5. **Aliasing no requiere DB vinculada:** puedes asignar un alias a cualquier
   perfil 1-4 antes de vincularlo.

## Arquitectura

### Almacenamiento — `internal/whatsapp/aliases.go` (nuevo)

Mapa alias persistido en un archivo dedicado, junto a las `whatsapp-N.db`:

```
~/Library/Application Support/nikte/wa_aliases.json
{ "2": "trabajo", "3": "personal" }
```

Llaves = número de perfil (string); valor = alias (preservando el case tal
como lo escribió el usuario). Vive en el dominio de WhatsApp (no en el
`config.json` de auth). Tipo en Go: `type Aliases map[int]string`.

Interfaz del paquete:

- `func LoadAliases() (Aliases, error)` — lee el archivo.
  - Archivo **ausente** → `(Aliases{}, nil)` (estado inicial normal).
  - Archivo **presente pero corrupto/ilegible** → `(nil, error)`.
  - Reads (status, resolución de `-p`) llaman e **ignoran** el error tratándolo
    como mapa vacío (`a, _ := LoadAliases()`), para nunca romper un comando.
  - Writes (`nk wa alias`) **abortan** si hay error, para no sobrescribir un
    archivo corrupto con un mapa vacío y borrar todos los alias.
- `func (a Aliases) Save() error` — escritura **atómica**: escribe a
  `wa_aliases.json.tmp` (0600) y `os.Rename` sobre el final. Evita archivos a
  medias. (No protege contra lost-update entre procesos concurrentes — ver YAGNI.)
- `func (a Aliases) AliasOf(profile int) string` — alias del perfil, o `""`.
- `func (a Aliases) ResolveAlias(name string) (int, bool)` — **determinista**:
  itera perfiles 1→4 en orden y devuelve el primero cuyo alias casa con `name`
  case-insensitive (tras `trim`). El orden fijo hace inequívoco incluso un
  archivo editado a mano con duplicados.
- `func (a Aliases) SetAlias(profile int, name string) Aliases` — semántica
  "mover": elimina `name` (case-insensitive) de **cualquier** otro perfil y lo
  asigna a `profile`. La unicidad es una **postcondición** de esta operación,
  no algo que valide `ValidateAlias`.
- `func (a Aliases) ClearAlias(profile int) Aliases` — quita el alias del perfil.
- `func ValidateAlias(name string) error` — reglas de **formato** (ver abajo);
  pura, unit-testeable. NO valida unicidad.

### Validación de alias (`ValidateAlias`)

Aplica solo a un nombre que se va a **asignar** (no al removerlo):

- Casa `^[A-Za-z0-9_-]{1,32}$` (alfanumérico, guion y guion-bajo; 1-32 chars).
- No puramente numérico (`^\d+$` prohibido) — evita ambigüedad con 1-4.
- No la palabra reservada `all` (case-insensitive).

### Resolución del perfil — `internal/whatsapp` + `internal/cli/wa.go`

El flag `-p` cambia de `Int` a `String`:
`waCmd.PersistentFlags().StringP("profile", "p", "1", "WhatsApp profile: 1-4 or an alias")`.

Nueva función de resolución en el paquete `whatsapp`:

```
func ResolveProfile(raw string) (int, error)
  1. raw = trim(raw); si parsea como entero → ValidateProfile(n) → n
  2. si no, LoadAliases() (ignora error→vacío).ResolveAlias(raw) → n
  3. si no, error: `unknown profile "<raw>" (use 1-4 or a defined alias; see "nk wa alias")`
```

En `internal/cli/wa.go`, un helper reemplaza **todas** las lecturas directas
del flag (hoy hay 5: link, send, ls, unlink, status):

```
func profileFromCmd(cmd *cobra.Command) (int, error) {
    raw, _ := cmd.Flags().GetString("profile")
    return whatsapp.ResolveProfile(raw)
}
```

`waCmd.PersistentPreRunE` llama a `profileFromCmd` y, si falla, aborta antes de
tocar cualquier DB/sesión. Cada `runWaX` también llama a `profileFromCmd` y
**verifica el error** (ya no `GetInt` ignorando el error, que daría perfil 0).

**Routing de `status` preservado:** `cmd.Flags().Changed("profile")` es
independiente del tipo del flag, así que con `StringP` sigue funcionando — sin
`-p` (default `"1"`, `Changed==false`) → overview; con `-p X` (`Changed==true`)
→ detalle. Se cubre con tests para `status` y `status -p 1`.

### Comando `nk wa alias`

```
nk wa alias                  # lista los 4 perfiles con su alias (o "—")
nk wa alias <profile> <name> # asigna alias (move-semantics)
nk wa alias <profile> ""     # quita el alias del perfil
nk wa alias --clear <profile># quita el alias del perfil
```

- `<profile>` se resuelve con `whatsapp.ResolveProfile`, así que puedes
  renombrar usando el alias viejo: `nk wa alias trabajo curro`.
- `<name>` que tras `trim` queda **vacío** → se trata como **remover** (NO pasa
  por `ValidateAlias`). Cualquier otro nombre pasa por `ValidateAlias`.
- Flujo write: `LoadAliases()` (aborta si error) → `SetAlias`/`ClearAlias` →
  `Save()`.

**PersistentPreRunE propio (no-op) para `aliasCmd`:** cobra ejecuta el
`PersistentPreRunE` más cercano. `aliasCmd` define el suyo (vacío) para que un
`-p` heredado e irrelevante (p. ej. `nk wa -p loquesea alias 2 trabajo`) no
bloquee la gestión de alias; el comando resuelve su `<profile>` posicional por
su cuenta. Los demás subcomandos siguen heredando el de `waCmd`.

### Salida con alias

Helper `profileLabel(profile int) string` → alias si existe (`"trabajo"`), si no
el número (`"2"`). Como los alias son `[A-Za-z0-9_-]`, las sugerencias de comando
quedan siempre shell-safe sin comillas.

- `nk wa status` (overview) muestra el alias por fila:
  ```
  WhatsApp profiles:
    1              Linked      (17608...@s.whatsapp.net)
    2 (trabajo)    Linked      (5217...@s.whatsapp.net)
    3 (personal)   Not linked
    4              Not linked
  ```
- `not linked` y confirmaciones de envío usan `profileLabel` en vez del número
  crudo (p. ej. `profile trabajo not linked. Run "nk wa link -p trabajo" first`).

## Flujo de uso

```
nk wa alias 2 trabajo           # nombra el perfil 2 (aunque aún no esté vinculado)
nk wa link -p trabajo           # vincula "trabajo" (escanea QR)
nk wa send -p trabajo 777... "Hola"
nk wa status                    # muestra "2 (trabajo)"
nk wa alias trabajo curro       # renombra (mueve el alias)
nk wa alias --clear 2           # quita el alias
nk wa send -p 2 777... "Hi"     # el número sigue funcionando siempre
```

## Manejo de errores

- `-p` con alias inexistente / número fuera de 1-4 → error de `ResolveProfile`
  antes de tocar nada.
- `nk wa alias` con nombre inválido → error de `ValidateAlias`, sin escribir.
- `wa_aliases.json` ausente → mapa vacío (comandos funcionan con números).
- `wa_aliases.json` corrupto → reads lo toleran (vacío); el comando `alias`
  **aborta** con error (no sobrescribe ni borra alias).

## Testing

- `ValidateAlias`: acepta `trabajo`, `work-phone`, `cuenta_2`; rechaza `""`,
  `"2"`, `"all"`/`"ALL"`, `"con espacio"`, `"emoji😀"`, string de 33 chars.
- `ResolveAlias` / `AliasOf`: round-trip; case-insensitive; perfil sin alias →
  `""`; determinismo con un mapa que tenga duplicados (gana el perfil menor).
- `SetAlias`: asignar un nombre ya usado por otro perfil lo **mueve** (no queda
  duplicado).
- `ClearAlias` y `nk wa alias <p> ""`: quitan el alias.
- `ResolveProfile`: `"2"`→2; `"trabajo"`→2 (con alias cargado); `"5"`→error;
  `"desconocido"`→error.
- `LoadAliases`: ausente → `({}, nil)`; corrupto → `(nil, error)`.
- `Save`: round-trip por archivo temporal + rename (sin dejar `.tmp`).
- `profileLabel`: con alias → alias; sin alias → número.
- `status` y `status -p 1` siguen ruteando a overview/detalle tras el cambio a
  `StringP`.

## Fuera de alcance (YAGNI)

- Protección contra lost-update entre procesos concurrentes (flock). El write
  atómico evita archivos corruptos; dos `nk wa alias` simultáneos son
  prácticamente imposibles en un CLI de un solo usuario.
- Más de 4 perfiles.
- Alias al vincular (`--name`).
- Persistir alias dentro de la `whatsapp-N.db`.
- Autocompletado de shell para alias.
