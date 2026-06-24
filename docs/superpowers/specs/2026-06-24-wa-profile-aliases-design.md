# Alias para perfiles de WhatsApp en `nk wa`

**Fecha:** 2026-06-24
**Estado:** Diseño aprobado, pre-implementación
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
4. **Reservados:** un alias no puede ser puramente numérico (ambigüedad con 1-4)
   ni la palabra `all` (la usa `nk wa ls --all`).

## Arquitectura

### Almacenamiento — `internal/whatsapp/aliases.go` (nuevo)

Mapa alias persistido en un archivo dedicado, junto a las `whatsapp-N.db`:

```
~/Library/Application Support/nikte/wa_aliases.json
{ "2": "trabajo", "3": "personal" }
```

Las llaves son el número de perfil (string), el valor el alias. Vive en el
dominio de WhatsApp (no en el `config.json` de auth).

Interfaz del paquete:

- `func LoadAliases() Aliases` — lee el archivo; si falta o está corrupto,
  devuelve un mapa vacío (resiliente, nunca rompe un comando). `Aliases` es
  `map[int]string` (perfil → alias).
- `func (a Aliases) Save() error` — escribe el archivo (modo 0600).
- `func (a Aliases) AliasOf(profile int) string` — alias del perfil, o `""`.
- `func (a Aliases) ResolveAlias(name string) (int, bool)` — busca un alias
  (case-insensitive, trim) y devuelve su número de perfil.
- `func ValidateAlias(name string) error` — aplica las reglas de validación
  (ver abajo). Pura, unit-testeable.

### Resolución del perfil — `internal/whatsapp` + `internal/cli/wa.go`

El flag `-p` cambia de `Int` a `String`:
`waCmd.PersistentFlags().StringP("profile", "p", "1", "WhatsApp profile: 1-4 or an alias")`.

Nueva función de resolución (reemplaza la lectura directa `GetInt("profile")`):

```
func ResolveProfile(raw string) (int, error)
  1. trim(raw); si parsea como entero → ValidateProfile(n) → n
  2. si no, LoadAliases().ResolveAlias(raw) → n
  3. si no, error: `unknown profile "<raw>" (use 1-4 or a defined alias; see "nk wa alias")`
```

`PersistentPreRunE` llama a `ResolveProfile` y, si falla, aborta antes de tocar
cualquier DB/sesión (igual que hoy con la validación 1-4). Cada `runWaX` obtiene
el `profile int` ya resuelto (helper `profileFromCmd(cmd) (int, error)`).

### Comando `nk wa alias`

```
nk wa alias                 # lista los 4 perfiles con su alias (o "—")
nk wa alias <profile> <name># asigna alias al perfil (profile = 1-4 o alias actual)
nk wa alias <profile> ""    # quita el alias del perfil
nk wa alias --clear <profile>
```

- Reasignar un nombre ya usado por otro perfil lo **mueve** (un alias apunta a
  un solo perfil; se elimina del anterior).
- `<profile>` se resuelve con `ResolveProfile`, así que puedes renombrar usando
  el alias viejo: `nk wa alias trabajo curro`.

### Reglas de validación de alias (`ValidateAlias`)

- No vacío tras `trim`.
- No puramente numérico (`^\d+$` prohibido) — evita ambigüedad con 1-4.
- No la palabra reservada `all` (case-insensitive).
- Largo ≤ 32 caracteres.
- Único entre perfiles (la reasignación lo mueve; no two perfiles con el mismo).

### Salida con alias

Los mensajes que hoy nombran el perfil por número usan un helper
`profileLabel(profile int) string` → `"trabajo"` si hay alias, si no `"2"`.

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
nk wa link -p 2                 # vincula el perfil 2 (escanea QR)
nk wa alias 2 trabajo           # ahora el perfil 2 = "trabajo"
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
- `wa_aliases.json` ausente/corrupto → mapa vacío (los comandos siguen
  funcionando con números).

## Testing

- `ValidateAlias`: acepta `trabajo`; rechaza `""`, `"2"`, `"all"`, `"ALL"`,
  string de 33 chars.
- `ResolveAlias` / `AliasOf`: round-trip; case-insensitive; perfil sin alias → `""`.
- `ResolveProfile`: `"2"`→2; `"trabajo"`→2 (con alias cargado); `"5"`→error;
  `"desconocido"`→error.
- Reasignación mueve el alias (no queda duplicado).
- `profileLabel`: con alias → alias; sin alias → número.

## Fuera de alcance (YAGNI)

- Más de 4 perfiles.
- Alias al vincular (`--name`).
- Persistir alias dentro de la `whatsapp-N.db`.
- Autocompletado de shell para alias.
