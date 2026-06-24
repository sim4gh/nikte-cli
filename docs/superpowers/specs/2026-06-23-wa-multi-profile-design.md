# Múltiples perfiles de WhatsApp en `nk wa`

**Fecha:** 2026-06-23
**Estado:** Diseño aprobado, pre-implementación

## Problema

El CLI `nk wa` soporta una sola cuenta de WhatsApp. La sesión vive en una base
SQLite fija (`whatsapp.db`). El usuario necesita manejar hasta **4 cuentas**
distintas con la misma funcionalidad (link, send, ls, status, unlink),
seleccionando el perfil por comando.

## Decisiones de diseño

1. **Selección de perfil:** flag global `-p, --profile N` en el comando `wa`,
   heredado por todos los subcomandos. Sin flag → perfil 1.
2. **Compatibilidad:** el perfil 1 sigue usando `whatsapp.db` tal cual (cero
   migración). Los perfiles 2-4 usan archivos nuevos.
3. **Rango:** 1-4. Fuera de rango → error claro, sin tocar ninguna DB.
4. **`status` sin `-p`:** muestra una vista general de los 4 perfiles leyendo
   solo la DB local (rápido, sin conectar). Con `-p N` verifica conexión viva
   de ese perfil (comportamiento actual).

## Arquitectura

### Capa `internal/whatsapp/client.go`

La ruta de la base se parametriza por perfil. Cada perfil es una base SQLite
independiente — sesión, dispositivo e historial totalmente aislados, sin estado
compartido.

```
GetDBPath(profile int) (string, error)
  profile 1     → <configDir>/whatsapp.db      (compat con la sesión existente)
  profile 2..4  → <configDir>/whatsapp-N.db
  fuera de 1..4 → error "profile must be between 1 and 4"
```

`configDir` no cambia: `~/Library/Application Support/nikte/` (darwin),
`%APPDATA%/nikte/` (windows), `$XDG_CONFIG_HOME/nikte/` (otros).

Firmas actualizadas para recibir `profile int`:

- `NewClient(profile int, verbose bool) (*whatsmeow.Client, error)`
- `IsLinked(profile int) bool`
- `DeleteDB(profile int) error` — borra `whatsapp-N.db`, `-wal` y `-shm`.

La validación de rango se centraliza en `GetDBPath`; el resto de funciones
propagan su error.

### Capa CLI `internal/cli/wa.go`

- Flag persistente en `waCmd`:
  `waCmd.PersistentFlags().IntP("profile", "p", 1, "WhatsApp profile (1-4)")`.
  Al ser persistente, lo heredan `link`, `send`, `ls`, `status`, `unlink`.
- Cada `runWaX` lee el perfil con `cmd.Flags().GetInt("profile")` y lo pasa a
  `whatsapp.NewClient(profile, …)` / `IsLinked(profile)` / `DeleteDB(profile)`.
- Los mensajes de error y de éxito que hoy dicen "WhatsApp not linked. Run
  \"nk wa link\"…" incluyen el perfil cuando es ≠ 1, p. ej.:
  `WhatsApp profile 2 not linked. Run "nk wa link -p 2" first`.
- El texto `Long` de `wa` y de `send` añade ejemplos con `-p`.

### Caso especial `status`

`runWaStatus` se bifurca según si el usuario pasó el flag, detectado con
`cmd.Flags().Changed("profile")`:

- **Con `-p N`:** comportamiento actual (estado + verificación de conexión viva)
  pero del perfil N.
- **Sin `-p`:** itera perfiles 1-4 y, **solo leyendo la DB local**, imprime una
  línea por perfil:

  ```
  WhatsApp profiles:
    1  Linked      (1525XXXXXXX)
    2  Linked      (5217XXXXXXX)
    3  Not linked
    4  Not linked
  ```

  El número/dispositivo sale de `client.Store.ID` (no requiere conectar). Para
  perfiles no vinculados se omite. Sin conexión de red → vista instantánea.

## Flujo de uso

```
nk wa link -p 2            # vincula la cuenta 2 (escanea QR)
nk wa send -p 2 7778887788 "Hola"
nk wa ls -p 2
nk wa status              # tabla de los 4 perfiles
nk wa status -p 2         # detalle + conexión viva del perfil 2
nk wa unlink -p 2         # borra solo whatsapp-2.db
nk wa send 7778887788 "Hi" # sin -p → perfil 1 (whatsapp.db)
```

## Manejo de errores

- `profile` fuera de 1-4: error antes de tocar ninguna DB.
- Perfil no vinculado: error que nombra el perfil y el comando correcto.
- `status` overview tolera perfiles inexistentes/corruptos: muestra
  "Not linked" en vez de abortar.

## Testing

- `GetDBPath`: perfil 1 → `whatsapp.db`; 2-4 → `whatsapp-N.db`; 0/5 → error.
- `IsLinked` / `DeleteDB` operan sobre el archivo correcto por perfil (aislados).
- Aislamiento: vincular el perfil 2 no altera `whatsapp.db` del perfil 1.

## Fuera de alcance (YAGNI)

- Etiquetas/nombres por perfil (se quedan numéricos 1-4).
- Perfil activo persistente (`use`/`switch`): se eligió flag por comando.
- Más de 4 perfiles.
