# Agente de balanza

Aplicación de escritorio para Windows que consume continuamente una balanza por puerto serial y envía tickets a una impresora POS ESC/POS serial.

## Configuración

Edita `config.yaml`:

- `serial.port`: puerto de la balanza, por ejemplo `COM3`.
- `parser.model`: usa `generic` para la balanza actual o `lexus_matrix_zero` para una Lexus Matrix Zero. Este perfil admite la trama normal (`= 126.35`) y la trama invertida de `Adr 99` (`=53.621`).
- `parser.pattern`: expresión regular adaptada a la trama enviada por la balanza. Debe contener el grupo `weight`; opcionalmente `stable` y `unit`.
- `printer.port`: puerto serial de la impresora, por ejemplo `COM4`.
- La pestaña **Configuración** permite editar estos valores y detectar los puertos COM disponibles.

La balanza y la impresora no pueden usar el mismo puerto. Si la impresora está instalada solamente mediante el spooler de Windows y no expone un COM, se necesita otro adaptador de impresión.

## Ejecución

```powershell
go run main.go
```

Se abrirá la ventana **Balanza POS**. La pestaña **Pesaje** muestra el peso en tiempo real y permite imprimir; la pestaña **Configuración** guarda los parámetros en `config.yaml`. Los cambios de conexión se aplican al reiniciar la aplicación.

Antes de imprimir, la aplicación muestra una vista previa con el número de ticket,
operador, producto, nota y peso. La numeración es correlativa y queda guardada en
la base local, por lo que no se reinicia al cerrar la aplicación. Nombre de empresa,
RUC, actividad, operador, título y pie se editan y guardan desde
**Configuración → Impresora POS**. El producto se selecciona desde un catálogo local
que incluye Cacao, Arroz y Café; desde la pantalla de pesaje se pueden agregar o
quitar productos y los cambios persisten al cerrar la aplicación.

### Ejecutar en segundo plano

Para mantener activa la balanza y la API local sin abrir la interfaz:

```powershell
./balanza-pos.exe --headless
```

También se puede activar mediante una variable de entorno:

```powershell
$env:BALANZA_HEADLESS = "true"
./balanza-pos.exe
```

En este modo no se crea ninguna ventana. El proceso continúa leyendo la balanza,
guardando las lecturas y publicando la API; sus mensajes se guardan en
`balanza-agent.log`. Para usarlo al iniciar Windows, crea una tarea programada con
el argumento `--headless` y establece como directorio de inicio la carpeta donde
están `balanza-pos.exe` y `config.yaml`.

Para usar otro archivo de configuración:

```powershell
$env:BALANZA_CONFIG = "config.produccion.yaml"
go run main.go
```

La aplicación reintenta abrir la balanza cada tres segundos cuando el puerto no está disponible.

## Crear el ejecutable

```powershell
go build -ldflags="-H windowsgui" -o balanza-pos.exe .
```

El manifiesto de Windows ya está integrado, por lo que el ejecutable no necesita archivos visuales adicionales.

## Consumir la balanza desde una aplicación web

Con el agente abierto, la API local queda disponible en `http://127.0.0.1:8765`:

```text
GET /api/v1/health   Estado de la API
GET /api/v1/reading Última lectura disponible
GET /api/v1/events  Flujo en tiempo real mediante Server-Sent Events
```

Lectura puntual:

```javascript
const reading = await fetch("http://127.0.0.1:8765/api/v1/reading")
  .then(response => response.json());
```

Actualización en tiempo real:

```javascript
const scale = new EventSource("http://127.0.0.1:8765/api/v1/events");
scale.onmessage = event => {
  const reading = JSON.parse(event.data);
  console.log(reading.weight, reading.unit);
};
```

Agrega el origen exacto de tu aplicación web en **Configuración → Integración web**. El ejemplo precargado es `http://127.0.0.1:5173/`. Por seguridad, la API escucha solamente en la computadora local de forma predeterminada.
