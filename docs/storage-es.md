***

## título: Almacenamiento&#xD;&#xA;sort_rank: 5

# Almacenamiento

Prometheus incluye una base de datos local de series temporales en disco, pero también se integra opcionalmente con sistemas de almacenamiento remoto.

## Almacenamiento local

La base de datos de series temporales locales de Prometheus almacena datos en un formato personalizado y altamente eficiente en el almacenamiento local.

### Diseño en disco

Las muestras ingeridas se agrupan en bloques de dos horas. Cada bloque de dos horas consta de
de un directorio que contiene un subdirectorio de fragmentos que contiene todos los ejemplos de series temporales
Para esa ventana de tiempo, un archivo de metadatos y un archivo de índice (que indiza los nombres de las métricas)
y etiquetas a series temporales en el directorio de fragmentos). Los ejemplos del directorio de fragmentos
se agrupan en uno o más archivos de segmento de hasta 512 MB cada uno de forma predeterminada. Cuando las series son
eliminados a través de la API, los registros de eliminación se almacenan en archivos de lápida separados (en su lugar)
de eliminar los datos inmediatamente de los segmentos de fragmentos).

El bloque actual para las muestras entrantes se mantiene en la memoria y no está completamente
Persistió. Está protegido contra bloqueos por un registro de escritura anticipada (WAL) que puede ser
se reproduce cuando se reinicia el servidor prometheus. Se almacenan los archivos de registro de escritura anticipada
En `wal` en segmentos de 128MB. Estos archivos contienen datos sin procesar que
aún no se ha compactado; por lo tanto, son significativamente más grandes que el bloque regular
Archivos. Prometheus conservará un mínimo de tres archivos de registro de escritura anticipada.
Los servidores de alto tráfico pueden retener más de tres archivos WAL para mantenerlos en
al menos dos horas de datos sin procesar.

El directorio de datos de un servidor Prometheus tiene un aspecto similar al siguiente:

    ./data
    ├── 01BKGV7JBM69T2G1BGBGM6KB12
    │   └── meta.json
    ├── 01BKGTZQ1SYQJTR4PB43C8PD98
    │   ├── chunks
    │   │   └── 000001
    │   ├── tombstones
    │   ├── index
    │   └── meta.json
    ├── 01BKGTZQ1HHWHV8FBJXW1Y3W0K
    │   └── meta.json
    ├── 01BKGV7JC0RY8A6MACW02A2PJD
    │   ├── chunks
    │   │   └── 000001
    │   ├── tombstones
    │   ├── index
    │   └── meta.json
    ├── chunks_head
    │   └── 000001
    └── wal
        ├── 000000002
        └── checkpoint.00000001
            └── 00000000

Tenga en cuenta que una limitación del almacenamiento local es que no está agrupado o
Replicado. Por lo tanto, no es arbitrariamente escalable o duradero frente a
interrupciones de unidad o nodo y debe administrarse como cualquier otro nodo único
base de datos. Se sugiere el uso de RAID para la disponibilidad del almacenamiento, y [Instantáneas](querying/api.md#snapshot)
se recomiendan para copias de seguridad. Con la
es posible retener años de datos en almacenamiento local.

Alternativamente, el almacenamiento externo se puede utilizar a través del [API de lectura/escritura remotas](https://prometheus.io/docs/operating/integrations/#remote-endpoints-and-storage). Se requiere una evaluación cuidadosa para estos sistemas, ya que varían mucho en durabilidad, rendimiento y eficiencia.

Para obtener más detalles sobre el formato de archivo, consulte [Formato TSDB](/tsdb/docs/format/README.md).

## Compactación

Los bloques iniciales de dos horas finalmente se compactan en bloques más largos en el fondo.

La compactación creará bloques más grandes que contienen datos que abarcan hasta el 10% del tiempo de retención, o 31 días, lo que sea menor.

## Aspectos operativos

Prometheus tiene varios indicadores que configuran el almacenamiento local. Los más importantes son:

*   `--storage.tsdb.path`: Donde Prometeo escribe su base de datos. El valor predeterminado es `data/`.
*   `--storage.tsdb.retention.time`: Cuándo eliminar datos antiguos. El valor predeterminado es `15d`. Reemplaza `storage.tsdb.retention` si este indicador se establece en cualquier otra cosa que no sea el predeterminado.
*   `--storage.tsdb.retention.size`: El número máximo de bytes de bloques de almacenamiento que se van a retener. Los datos más antiguos se eliminarán primero. El valor predeterminado es `0` o deshabilitado. Unidades soportadas: B, KB, MB, GB, TB, PB, EB. Ej: "512MB". Basado en potencias de 2, por lo que 1KB es 1024B. Solo se eliminan los bloques persistentes para respetar esta retención, aunque los fragmentos WAL y m-mapeados se cuentan en el tamaño total. Por lo tanto, el requisito mínimo para el disco es el espacio máximo ocupado por el `wal` (el WAL y el punto de control) y `chunks_head` (m-mapped Head chunks) directorio combinado (picos cada 2 horas).
*   `--storage.tsdb.retention`: Obsoleto a favor de `storage.tsdb.retention.time`.
*   `--storage.tsdb.wal-compression`: permite la compresión del registro de escritura anticipada (WAL). Dependiendo de sus datos, puede esperar que el tamaño de WAL se reduzca a la mitad con poca carga adicional de cpu. Este indicador se introdujo en 2.11.0 y se habilitó de forma predeterminada en 2.20.0. Tenga en cuenta que una vez habilitado, degradar Prometheus a una versión inferior a 2.11.0 requerirá eliminar el WAL.

Prometheus almacena un promedio de solo 1-2 bytes por muestra. Por lo tanto, para planificar la capacidad de un servidor Prometheus, puede utilizar la fórmula aproximada:

    needed_disk_space = retention_time_seconds * ingested_samples_per_second * bytes_per_sample

Para reducir la tasa de muestras ingeridas, puede reducir el número de series temporales que raspa (menos objetivos o menos series por objetivo), o puede aumentar el intervalo de raspado. Sin embargo, reducir el número de series es probablemente más efectivo, debido a la compresión de muestras dentro de una serie.

Si su almacenamiento local se corrompe por cualquier razón, lo mejor
La estrategia para abordar el problema es cerrar Prometheus y luego eliminar el
directorio de almacenamiento completo. También puede intentar eliminar directorios de bloques individuales,
o el directorio WAL para resolver el problema.  Tenga en cuenta que esto significa perder
aproximadamente dos horas de datos por directorio de bloques. De nuevo, el local de Prometheus
el almacenamiento no está destinado a ser un almacenamiento duradero a largo plazo; soluciones externas
ofrecen una retención extendida y durabilidad de los datos.

PRECAUCIÓN: Los sistemas de archivos no compatibles con POSIX no son compatibles con el almacenamiento local de Prometheus, ya que pueden producirse daños irrecuperables. Los sistemas de archivos NFS (incluido efS de AWS) no son compatibles. NFS podría ser compatible con POSIX, pero la mayoría de las implementaciones no lo son. Se recomienda encarecidamente utilizar un sistema de archivos local para mayor fiabilidad.

Si se especifican las políticas de retención de tiempo y tamaño, lo que se active primero
se utilizará.

La limpieza de bloques caducados se realiza en segundo plano. Puede tomar hasta dos horas eliminar los bloques caducados. Los bloques deben estar completamente caducados antes de que se eliminen.

## Integraciones de almacenamiento remoto

El almacenamiento local de Prometheus se limita a la escalabilidad y durabilidad de un solo nodo.
En lugar de tratar de resolver el almacenamiento en clúster en Prometheus, Prometheus ofrece
un conjunto de interfaces que permiten integrarse con sistemas de almacenamiento remoto.

### Visión general

Prometheus se integra con los sistemas de almacenamiento remoto de tres maneras:

*   Prometheus puede escribir muestras que ingiere en una URL remota en un formato estandarizado.
*   Prometheus puede recibir muestras de otros servidores Prometheus en un formato estandarizado.
*   Prometheus puede leer (atrás) datos de muestra de una URL remota en un formato estandarizado.

![Remote read and write architecture](images/remote_integrations.png)

Los protocolos de lectura y escritura utilizan una codificación de búfer de protocolo comprimida rápidamente a través de HTTP. Los protocolos aún no se consideran API estables y pueden cambiar para usar gRPC a través de HTTP / 2 en el futuro, cuando se pueda suponer que todos los saltos entre Prometheus y el almacenamiento remoto admiten HTTP / 2.

Para obtener más información sobre la configuración de integraciones de almacenamiento remoto en Prometheus, consulte el [escritura remota](configuration/configuration.md#remote_write) y [lectura remota](configuration/configuration.md#remote_read) de la documentación de configuración de Prometheus.

El receptor de escritura remota incorporado se puede habilitar configurando el `--web.enable-remote-write-receiver` indicador de línea de comandos. Cuando está habilitado, el extremo del receptor de escritura remota es `/api/v1/write`.

Para obtener más información sobre los mensajes de solicitud y respuesta, consulte el [definiciones de búfer de protocolo de almacenamiento remoto](https://github.com/prometheus/prometheus/blob/main/prompb/remote.proto).

Tenga en cuenta que en la ruta de lectura, Prometheus solo obtiene datos de series sin procesar para un conjunto de selectores de etiquetas y rangos de tiempo desde el extremo remoto. Toda la evaluación de PromQL en los datos sin procesar todavía ocurre en el propio Prometheus. Esto significa que las consultas de lectura remota tienen cierto límite de escalabilidad, ya que todos los datos necesarios deben cargarse primero en el servidor de Prometheus de consulta y luego procesarse allí. Sin embargo, el apoyo a la evaluación totalmente distribuida de PromQL se consideró inviable por el momento.

### Integraciones existentes

Para obtener más información sobre las integraciones existentes con sistemas de almacenamiento remoto, consulte el [Documentación de integraciones](https://prometheus.io/docs/operating/integrations/#remote-endpoints-and-storage).

## Relleno desde formato OpenMetrics

### Visión general

Si un usuario desea crear bloques en el TSDB a partir de datos que se encuentran en [OpenMetrics](https://openmetrics.io/) formatear, pueden hacerlo usando relleno. Sin embargo, deben tener cuidado y tener en cuenta que no es seguro rellenar los datos de las últimas 3 horas (el bloque de cabeza actual) ya que este rango de tiempo puede superponerse con el bloque de cabeza actual Prometheus todavía está mutando. El relleno creará nuevos bloques TSDB, cada uno con dos horas de datos de métricas. Esto limita los requisitos de memoria de la creación de bloques. La compactación de los bloques de dos horas en bloques más grandes es realizada más tarde por el propio servidor Prometheus.

Un caso de uso típico es migrar datos de métricas de un sistema de monitoreo diferente o una base de datos de series temporales a Prometheus. Para ello, el usuario debe convertir primero los datos de origen en [OpenMetrics](https://openmetrics.io/)  formato, que es el formato de entrada para el relleno como se describe a continuación.

### Uso

El relleno se puede utilizar a través de la línea de comandos de Promtool. Promtool escribirá los bloques en un directorio. De forma predeterminada, este directorio de salida es ./data/, puede cambiarlo utilizando el nombre del directorio de salida deseado como argumento opcional en el subcomando.

    promtool tsdb create-blocks-from openmetrics <input file> [<output directory>]

Después de la creación de los bloques, muévalo al directorio de datos de Prometheus. Si hay una superposición con los bloques existentes en Prometheus, la bandera `--storage.tsdb.allow-overlapping-blocks` necesita ser configurado. Tenga en cuenta que los datos rellenados están sujetos a la retención configurada para su servidor Prometheus (por tiempo o tamaño).

#### Duraciones de bloque más largas

De forma predeterminada, la herramienta de protección utilizará la duración predeterminada del bloque (2h) para los bloques; este comportamiento es el más generalmente aplicable y correcto. Sin embargo, al rellenar datos en un largo rango de tiempos, puede ser ventajoso usar un valor mayor para la duración del bloque para rellenar más rápido y evitar compactaciones adicionales por parte de TSDB más adelante.

El `--max-block-duration` flag permite al usuario configurar una duración máxima de bloques. La herramienta de relleno elegirá una duración de bloque adecuada no mayor que esta.

Si bien los bloques más grandes pueden mejorar el rendimiento del relleno de grandes conjuntos de datos, también existen inconvenientes. Las políticas de retención basadas en el tiempo deben mantener todo el bloque si incluso una muestra del bloque (potencialmente grande) todavía está dentro de la política de retención. Por el contrario, las políticas de retención basadas en el tamaño eliminarán todo el bloque, incluso si el TSDB solo supera el límite de tamaño de una manera menor.

Por lo tanto, el relleno con pocos bloques, eligiendo así una mayor duración del bloque, debe hacerse con cuidado y no se recomienda para ninguna instancia de producción.

## Relleno para reglas de grabación

### Visión general

Cuando se crea una nueva regla de grabación, no hay datos históricos para ella. Los datos de la regla de registro solo existen a partir del momento de la creación. `promtool` permite crear datos de reglas de registro histórico.

### Uso

Para ver todas las opciones, utilice: `$ promtool tsdb create-blocks-from rules --help`.

Ejemplo de uso:

    $ promtool tsdb create-blocks-from rules \
        --start 1617079873 \
        --end 1617097873 \
        --url http://mypromserver.com:9090 \
        rules.yaml rules2.yaml

Los archivos de reglas de grabación proporcionados deben ser normales [Archivo de reglas de Prometheus](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/).

El resultado de `promtool tsdb create-blocks-from rules` es un directorio que contiene bloques con los datos históricos de las reglas para todas las reglas de los archivos de reglas de grabación. De forma predeterminada, el directorio de salida es `data/`. Para hacer uso de estos nuevos datos de bloque, los bloques deben moverse a un directorio de datos de instancia de Prometheus en ejecución `storage.tsdb.path` que tiene la bandera `--storage.tsdb.allow-overlapping-blocks` Habilitado. Una vez movidos, los nuevos bloques se fusionarán con los bloques existentes cuando se ejecute la siguiente compactación.

### Limitaciones

*   Si ejecuta el backfiller de reglas varias veces con las horas de inicio/finalización superpuestas, se crearán bloques que contengan los mismos datos cada vez que se ejecute el backfiller de reglas.
*   Se evaluarán todas las reglas de los archivos de reglas de grabación.
*   Si el `interval` se establece en el archivo de regla de grabación que tendrá prioridad sobre el `eval-interval` en el comando de relleno de reglas.
*   Actualmente, las alertas se omiten si se encuentran en el archivo de reglas de grabación.
*   Las reglas del mismo grupo no pueden ver los resultados de las reglas anteriores. Lo que significa que las reglas que se refieren a otras reglas que se rellenan no son compatibles. Una solución alternativa consiste en rellenar varias veces y crear primero los datos dependientes (y mover primero los datos dependientes al directorio de datos del servidor prometheus para que sea accesible desde la API de Prometheus).
