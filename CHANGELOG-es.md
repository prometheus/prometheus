# Registro de cambios

## 2.36.1 / 2022-06-09

*   \[BUGFIX] promtool: Agregar opción --lint-fatal #10840

## 2.36.0 / 2022-05-30

*   \[CARACTERÍSTICA] Agregue la acción de reetiquetado en minúsculas y mayúsculas. #10641
*   \[CARACTERÍSTICA] SD: Agregue la integración de IONOS Cloud. #10514
*   \[CARACTERÍSTICA] SD: Añadir integración Vultr. #10714
*   \[CARACTERÍSTICA] SD: Agregue la métrica de recuento de fallas de Linode SD. #10673
*   \[CARACTERÍSTICA] Agregue prometheus_ready métrica. #10682
*   \[MEJORA] Agregue stripDomain a la función de plantilla. #10475
*   \[MEJORA] Interfaz de usuario: habilite la búsqueda activa a través de destinos eliminados. #10668
*   \[MEJORA] promtool: admite emparejadores al consultar valores de etiquetas. #10727
*   \[MEJORA] Agregue el identificador de modo de agente. #9638
*   \[CORRECCIÓN DE ERRORES] Cambiar TotalQueryableSamples de int a int64. #10549
*   \[BUGFIX] tsdb/agent: Ignorar ejemplos duplicados. #10595
*   \[CORRECCIÓN DE ERRORES] TSDB: Corrija el desbordamiento de trozos que agrega muestras a una velocidad variable. #10607
*   \[CORRECCIÓN DE ERRORES] Detenga el administrador de reglas antes de que se detenga TSDB. #10680

## 2.35.0 / 2022-04-21

Esta versión de Prometheus está construida con go1.18, que contiene dos cambios notables relacionados con TLS:

1.  [TLS 1.0 y 1.1 deshabilitados de forma predeterminada en el lado del cliente](https://go.dev/doc/go1.18#tls10).
    Los usuarios de Prometheus pueden anular esto con el `min_version` parámetro de [tls_config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#tls_config).
2.  [Se rechazan los certificados firmados con la función hash SHA-1](https://go.dev/doc/go1.18#sha1). Esto no se aplica a los certificados raíz autofirmados.

*   \[CAMBIAR] TSDB: Eliminar `*.tmp` WAL cuando se inicia Prometheus. #10317
*   \[CHANGE] promtool: Agregar nueva bandera `--lint` (habilitado de forma predeterminada) para los comandos `check rules` y `check config`, lo que da como resultado un nuevo código de salida (`3`) para errores de linter. #10435
*   \[CARACTERÍSTICA] Soporte para configurar automáticamente la variable `GOMAXPROCS` al límite de CPU del contenedor. Habilitar con el indicador `--enable-feature=auto-gomaxprocs`. #10498
*   \[CARACTERÍSTICA] PromQL: Amplíe las estadísticas con el número total y máximo de muestras en una consulta. Además, las estadísticas por paso están disponibles con --enable-feature=promql-per-step-stats y usando `stats=all` en la API de consulta.
    Habilitar con el indicador `--enable-feature=per-step-stats`. #10369
*   \[MEJORA] Prometheus está construido con Go 1.18. #10501
*   \[MEJORA] TSDB: clasificación más eficiente de las publicaciones leídas de WAL en el inicio. #10500
*   \[MEJORA] Azure SD: agregue una métrica para realizar un seguimiento de los errores de Azure SD. #10476
*   \[MEJORA] Azure SD: Agregar un opcional `resource_group` configuración. #10365
*   \[MEJORA] Kubernetes SD: Soporte `discovery.k8s.io/v1` `EndpointSlice` (anteriormente sólo `discovery.k8s.io/v1beta1` `EndpointSlice` fue apoyado). #9570
*   \[MEJORA] Kubernetes SD: permite adjuntar metadatos de nodo a pods detectados. #10080
*   \[MEJORA] OAuth2: Compatibilidad con el uso de una URL de proxy para obtener tokens de OAuth2. #10492
*   \[MEJORA] Configuración: Agregue la capacidad de deshabilitar HTTP2. #10492
*   \[MEJORA] Configuración: Admite la anulación de la versión mínima de TLS. #10610
*   \[CORRECCIÓN DE ERRORES] Kubernetes SD: Incluye explícitamente la autenticación gcp de k8s.io. #10516
*   \[CORRECCIÓN DE ERRORES] Corrija el analizador OpenMetrics para ordenar correctamente las etiquetas en mayúsculas. #10510
*   \[CORRECCIÓN DE ERRORES] Interfaz de usuario: Corrija la información sobre herramientas sobre el intervalo de raspado y la duración que no se muestra en la página de destino. #10545
*   \[CORRECCIÓN DE ERRORES] Seguimiento/GRPC: Establezca las credenciales de TLS solo cuando la inseguridad sea falsa. #10592
*   \[CORRECCIÓN DE ERRORES] Agente: Corrija la colisión de ID al cargar un WAL con varios segmentos. #10587
*   \[CORRECCIÓN DE ERRORES] Escritura remota: solucione un punto muerto entre el lote y el vaciado de la cola. #10608

## 2.34.0 / 2022-03-15

*   \[CAMBIAR] Interfaz de usuario: se ha eliminado la interfaz de usuario clásica. #10208
*   \[CAMBIAR] Rastreo: Migre de Jaeger a rastreo basado en OpenTelemetry. #9724, #10203, #10276
*   \[MEJORA] TSDB: Deshabilite la cola de escritura de fragmentos de forma predeterminada y permita la configuración con el indicador experimental `--storage.tsdb.head-chunks-write-queue-size`. #10425
*   \[MEJORA] HTTP SD: Agregue un contador de errores. #10372
*   \[MEJORA] Azure SD: establezca el agente de usuario de Prometheus en las solicitudes. #10209
*   \[MEJORA] Uyuni SD: Reducir el número de inicios de sesión en Uyuni. #10072
*   \[MEJORA] Scrape: Registra cuando se encuentra un tipo de medio no válido durante un scrape. #10186
*   \[MEJORA] Scrape: Acepte application/openmetrics-text;version=1.0.0 además de version=0.0.1. #9431
*   \[MEJORA] Lectura remota: agregue una opción para no usar etiquetas externas como selectores para la lectura remota. #10254
*   \[MEJORA] INTERFAZ DE USUARIO: Optimiza la página de alertas y agrega una barra de búsqueda. #10142
*   \[MEJORA] Interfaz de usuario: mejora los colores de los gráficos que eran difíciles de ver. #10179
*   \[MEJORA] Configuración: Permitir el escape de `$` con `$$` cuando se utilizan variables de entorno con etiquetas externas. #10129
*   \[CORRECCIÓN DE ERRORES] PromQL: Devuelve correctamente un error de histogram_quantile cuando las métricas tienen el mismo conjunto de etiquetas. #10140
*   \[CORRECCIÓN DE ERRORES] INTERFAZ DE USUARIO: Corrige un error que establece la entrada de rango en la resolución. #10227
*   \[CORRECCIÓN DE ERRORES] TSDB: Corregir un pánico de consulta cuando `memory-snapshot-on-shutdown` está habilitado. #10348
*   \[CORRECCIÓN DE ERRORES] Analizador: especifique el tipo de errores del analizador de metadatos. #10269
*   \[CORRECCIÓN DE ERRORES] Raspado: Arregle los cambios de límite de etiqueta que no se aplican. #10370

## 2.33.5 / 2022-03-08

Los binarios publicados con esta versión se crean con Go1.17.8 para evitar [CVE-2022-24921 (en inglés)](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2022-24921).

*   \[CORRECCIÓN DE ERRORES] Escritura remota: solucione el punto muerto entre agregar a la cola y obtener el lote. #10395

## 2.33.4 / 2022-02-22

*   \[CORRECCIÓN DE ERRORES] TSDB: Solucione el pánico cuando m-mapping head chunks en el disco. #10316

## 2.33.3 / 2022-02-11

*   \[CORRECCIÓN DE ERRORES] Azure SD: corrija una regresión cuando la dirección IP pública no está establecida. #10289

## 2.33.2 / 2022-02-11

*   \[CORRECCIÓN DE ERRORES] Azure SD: solucione el pánico cuando la dirección IP pública no está configurada. #10280
*   \[CORRECCIÓN DE ERRORES] Escritura remota: solucione el interbloqueo al detener un fragmento. #10279

## 2.33.1 / 2022-02-02

*   \[CORRECCIÓN DE ERRORES] SD: Arreglar *no existe tal archivo o directorio* en K8s SD cuando no se ejecuta dentro de K8s. #10235

## 2.33.0 / 2022-01-29

*   \[CAMBIAR] PromQL: Promover la compensación negativa y `@` modifer a características estables. #10121
*   \[CAMBIAR] Web: Promueva el receptor de escritura remota a estable. #10119
*   \[CARACTERÍSTICA] Configuración: Agregar `stripPort` función de plantilla. #10002
*   \[CARACTERÍSTICA] Promtool: Agregar análisis de cardinalidad a `check metrics`, habilitado por el indicador `--extended`. #10045
*   \[CARACTERÍSTICA] SD: habilite la detección de destinos en su propio espacio de nombres K8s. #9881
*   \[CARACTERÍSTICA] SD: Agregue la etiqueta de identificación del proveedor en K8s SD. #9603
*   \[CARACTERÍSTICA] Web: agregue un campo de límite a la API de reglas. #10152
*   \[MEJORA] Escritura remota: Evite las asignaciones almacenando en búfer estructuras concretas en lugar de interfaces. #9934
*   \[MEJORA] Escritura remota: registre los detalles de la serie temporal para muestras fuera de orden en el receptor de escritura remota. #9894
*   \[MEJORA] Escritura remota: fragmentar más cuando está atrasado. #9274
*   \[MEJORA] TSDB: Utilice una clave de mapa más simple para mejorar el rendimiento de ingesta ejemplar. #10111
*   \[MEJORA] TSDB: Evite las asignaciones al salir del montón de publicaciones intersectadas. #10092
*   \[MEJORA] TSDB: Haga que la escritura de fragmentos no se bloquee, evitando picos de latencia en la escritura remota. #10051
*   \[MEJORA] TSDB: Mejore el rendimiento de coincidencia de etiquetas. #9907
*   \[MEJORA] Interfaz de usuario: optimiza la página de detección de servicios y agrega una barra de búsqueda. #10131
*   \[MEJORA] UI: Optimiza la página de destino y agrega una barra de búsqueda. #10103
*   \[CORRECCIÓN DE ERRORES] Promtool: Haga que los códigos de salida sean más consistentes. #9861
*   \[CORRECCIÓN DE ERRORES] Promtool: Corrige la descamación de las pruebas de reglas. #8818
*   \[CORRECCIÓN DE ERRORES] Escritura remota: Actualización `prometheus_remote_storage_queue_highest_sent_timestamp_seconds` métrica cuando la escritura falla irrecuperablemente. #10102
*   \[CORRECCIÓN DE ERRORES] Almacenamiento: Evite el pánico en `BufferedSeriesIterator`. #9945
*   \[CORRECCIÓN DE ERRORES] TSDB: CompactBlockMetas debe producir menta / maxt correcto para bloques superpuestos. #10108
*   \[CORRECCIÓN DE ERRORES] TSDB: Corregir el registro de tamaño de almacenamiento ejemplar. #9938
*   \[CORRECCIÓN DE ERRORES] Interfaz de usuario: Corrija los destinos de clic superpuestos para las casillas de verificación del estado de alerta. #10136
*   \[CORRECCIÓN DE ERRORES] INTERFAZ DE USUARIO: Corrección *Insalubre* filtrar en la página de destino para mostrar solo *Insalubre* Objetivos. #10103
*   \[CORRECCIÓN DE ERRORES] INTERFAZ DE USUARIO: Corrige el autocompletado cuando la expresión está vacía. #10053
*   \[CORRECCIÓN DE ERRORES] TSDB: Solucione el interbloqueo de GC y escritura simultáneos. #10166

## 2.32.1 / 2021-12-17

*   \[CORRECCIÓN DE ERRORES] Scrape: corrige las métricas de informes cuando se alcanza el límite de muestra durante el informe. #9996
*   \[CORRECCIÓN DE ERRORES] Raspado: Asegúrese de que el intervalo de raspado y el tiempo de espera de raspado estén siempre establecidos. #10023
*   \[CORRECCIÓN DE ERRORES] TSDB: Exponer y corregir errores en iteradores `Seek()` método. #10030

## 2.32.0 / 2021-12-09

Esta versión presenta el Agente Prometheus, un nuevo modo de funcionamiento para
Prometheus optimizado para escenarios de escritura remota solamente. En este modo, Prometeo
no genera bloques en el sistema de archivos local y no se puede consultar localmente.
Habilitar con `--enable-feature=agent`.

Obtenga más información sobre el agente Prometheus en nuestro [entrada de blog](https://prometheus.io/blog/2021/11/16/agent/).

*   \[CAMBIAR] Escritura remota: cambie el tiempo máximo de reintento predeterminado de 100 ms a 5 segundos. #9634
*   \[CARACTERÍSTICA] Agente: Nuevo modo de operación optimizado para escenarios de solo escritura remota, sin almacenamiento local. Habilitar con `--enable-feature=agent`. #8785 #9851 #9664 #9939 #9941 #9943
*   \[CARACTERÍSTICA] Promtool: Añadir `promtool check service-discovery` mandar. #8970
*   \[CARACTERÍSTICA] INTERFAZ DE USUARIO: Agrega búsqueda en el menú desplegable de métricas. #9629
*   \[CARACTERÍSTICA] Plantillas: Agregue parseDuration a las funciones de plantilla. #8817
*   \[MEJORA] Promtool: Mejore la salida de la prueba. #8064
*   \[MEJORA] PromQL: Utilice la suma kahan para una mejor estabilidad numérica. #9588
*   \[MEJORA] Escritura remota: Reutilice la memoria para la clasificación. #9412
*   \[MEJORA] Raspar: Añadir `scrape_body_size_bytes` métrica de raspado detrás de la `--enable-feature=extra-scrape-metrics` bandera. #9569
*   \[MEJORA] TSDB: Agregue compatibilidad con Windows arm64. #9703
*   \[MEJORA] TSDB: Optimice la consulta omitiendo la ordenación innecesaria en TSDB. #9673
*   \[MEJORA] Plantillas: Admite int y uint como tipos de datos para el formato de plantillas. #9680
*   \[MEJORA] INTERFAZ DE USUARIO: Preferir `rate` sobre `rad`, `delta` sobre `deg`y `count` sobre `cos` en autocompletar. #9688
*   \[MEJORA] Linode SD: Ajuste los tamaños de página de solicitud de API. #9779
*   \[CORRECCIÓN DE ERRORES] TSDB: agregue más comprobaciones de tamaño al escribir secciones individuales en el índice. #9710
*   \[CORRECCIÓN DE ERRORES] PromQL: Hacer `deriv()` devuelve valores cero para series constantes. #9728
*   \[CORRECCIÓN DE ERRORES] TSDB: Solucione el pánico cuando el directorio del punto de control está vacío. #9687
*   \[CORRECCIÓN DE ERRORES] TSDB: Solucione el pánico, los trozos fuera de orden y la advertencia de carrera durante la repetición de WAL. #9856
*   \[CORRECCIÓN DE ERRORES] Interfaz de usuario: Representar correctamente los vínculos para destinos con direcciones IPv6 que contienen un ID de zona. #9853
*   \[CORRECCIÓN DE ERRORES] Promtool: Corrección de comprobación de `authorization.credentials_file` y `bearer_token_file` Campos. #9883
*   \[CORRECCIÓN DE ERRORES] Uyuni SD: Corregir la excepción de puntero nulo durante la inicialización. #9924 #9950
*   \[CORRECCIÓN DE ERRORES] TSDB: Corrija las consultas después de una reproducción de instantáneas fallida. #9980

## 2.31.2 / 2021-12-09

*   \[CORRECCIÓN DE ERRORES] TSDB: Corrija las consultas después de una reproducción de instantáneas fallida. #9980

## 2.31.1 / 2021-11-05

*   \[CORRECCIÓN DE ERRORES] SD: Solucione un pánico cuando el administrador de descubrimiento experimental recibe
    objetivos durante una recarga. #9656

## 2.31.0 / 2021-11-02

*   \[CAMBIAR] INTERFAZ DE USUARIO: Elimine el editor PromQL estándar en favor del editor basado en codemirror. #9452
*   \[CARACTERÍSTICA] PromQL: Añadir funciones trigonométricas y `atan2` operador binario. #9239 #9248 #9515
*   \[CARACTERÍSTICA] Remoto: agregue compatibilidad con ejemplos en el extremo del receptor de escritura remota. #9319 #9414
*   \[CARACTERÍSTICA] SD: Agregar detección de servicios PuppetDB. #8883
*   \[CARACTERÍSTICA] SD: Agregue la detección de servicios de Uyuni. #8190
*   \[CARACTERÍSTICA] Web: Agregue compatibilidad con encabezados HTTP relacionados con la seguridad. #9546
*   \[MEJORA] Azure SD: Agregar `proxy_url`, `follow_redirects`, `tls_config`. #9267
*   \[MEJORA] Relleno: Agregar `--max-block-duration` en `promtool create-blocks-from rules`. #9511
*   \[MEJORA] Configuración: Imprima tamaños legibles por humanos con unidades en lugar de números brutos. #9361
*   \[MEJORA] HTTP: Vuelva a habilitar HTTP/2. #9398
*   \[MEJORA] Kubernetes SD: Avise al usuario si el número de puntos finales excede el límite. #9467
*   \[MEJORA] OAuth2: Agregue la configuración de TLS a las solicitudes de token. #9550
*   \[MEJORA] PromQL: Varias optimizaciones. #9365 #9360 #9362 #9552
*   \[MEJORA] PromQL: Hacer que las agregaciones sean deterministas en consultas instantáneas. #9459
*   \[MEJORA] Reglas: Agregue la capacidad de limitar el número de alertas o series. #9260 #9541
*   \[MEJORA] SD: Gestor de descubrimiento experimental para evitar reinicios al volver a cargar. Deshabilitado de forma predeterminada, habilitar con indicador `--enable-feature=new-service-discovery-manager`. #9349 #9537
*   \[MEJORA] INTERFAZ DE USUARIO: Cambios en la configuración del intervalo de tiempo de rebote. #9359
*   \[CORRECCIÓN DE ERRORES] Relleno: Aplicar etiquetas de regla después de las etiquetas de consulta. #9421
*   \[CORRECCIÓN DE ERRORES] Scrape: resuelve conflictos entre varios prefijos de etiqueta exportados. #9479 #9518
*   \[CORRECCIÓN DE ERRORES] Scrape: Reinicie los bucles de scrape cuando `__scrape_interval__` se cambia. #9551
*   \[CORRECCIÓN DE ERRORES] TSDB: Corregir la pérdida de memoria en la eliminación de muestras. #9151
*   \[CORRECCIÓN DE ERRORES] Interfaz de usuario: Use un margen inferior coherente para todos los tipos de alerta. #9318

## 2.30.4 / 2021-12-09

*   \[CORRECCIÓN DE ERRORES] TSDB: Corrija las consultas después de una reproducción de instantáneas fallida. #9980

## 2.30.3 / 2021-10-05

*   \[CORRECCIÓN DE ERRORES] TSDB: Solucione el pánico en la reproducción de instantáneas fallidas. #9438
*   \[CORRECCIÓN DE ERRORES] TSDB: No falle la reproducción de instantáneas con el almacenamiento ejemplar deshabilitado cuando la instantánea contiene ejemplos. #9438

## 2.30.2 / 2021-10-01

*   \[CORRECCIÓN DE ERRORES] TSDB: No cometa errores al superponer fragmentos mapeados m durante la reproducción de WAL. #9381

## 2.30.1 / 2021-09-28

*   \[MEJORA] Escritura remota: redacte la URL de escritura remota cuando se usa para la etiqueta de métrica. #9383
*   \[MEJORA] Interfaz de usuario: Redactar contraseñas de URL de escritura remota y URL de proxy en el `/config` página. #9408
*   \[BUGFIX] relleno de reglas de promtool: Evite la creación de datos antes de la hora de inicio. #9339
*   \[BUGFIX] relleno de reglas de promtool: No consultar después de la hora de finalización. #9340
*   \[CORRECCIÓN DE ERRORES] Azure SD: solucione el pánico cuando no se establece ningún nombre de equipo. #9387

## 2.30.0 / 2021-09-14

*   \[CARACTERÍSTICA] **experimental** TSDB: Instantáneas en memoria de fragmentos en apagado para reinicios más rápidos. Atrás `--enable-feature=memory-snapshot-on-shutdown` bandera. #7229
*   \[CARACTERÍSTICA] **experimental** Scrape: Configure el intervalo de raspado y el tiempo de espera de raspado mediante el reetiquetado mediante el uso `__scrape_interval__` y `__scrape_timeout__` etiquetas respectivamente. #8911
*   \[CARACTERÍSTICA] Raspar: Añadir `scrape_timeout_seconds` y `scrape_sample_limit` métrico. Atrás `--enable-feature=extra-scrape-metrics` para evitar la cardinalidad adicional de forma predeterminada. #9247 #9295
*   \[MEJORA] Raspar: Añadir `--scrape.timestamp-tolerance` para ajustar la tolerancia de marca de tiempo de raspado cuando está habilitada a través de `--scrape.adjust-timestamps`. #9283
*   \[MEJORA] Escritura remota: mejore el rendimiento al enviar ejemplos. #8921
*   \[MEJORA] TSDB: Optimice la carga de WAL eliminando el mapa adicional y almacenando en caché el tiempo mínimo #9160
*   \[MEJORA] promtool: Acelere la comprobación de reglas duplicadas. #9262/#9306
*   \[MEJORA] Scrape: Reduce las asignaciones al analizar las métricas. #9299
*   \[MEJORA] docker_sd: Admite el modo de red host #9125
*   \[CORRECCIÓN DE ERRORES] Ejemplos: Solucione el pánico al cambiar el tamaño del almacenamiento ejemplar de 0 a un tamaño distinto de cero. #9286
*   \[CORRECCIÓN DE ERRORES] TSDB: Disminución correcta `prometheus_tsdb_head_active_appenders` cuando el apéndice no tiene muestras. #9230
*   \[BUGFIX] relleno de reglas de promtool: Devuelva 1 si el relleno no se realizó correctamente. #9303
*   \[BUGFIX] relleno de reglas de promtool: Evite la creación de bloques superpuestos. #9324
*   Configuración de \[BUGFIX]: Solucione un pánico al volver a cargar la configuración con un `null` acción de reetiquetado. #9224

## 2.29.2 / 2021-08-27

*   \[CORRECCIÓN DE ERRORES] Corrija Kubernetes SD al no descubrir Ingress en Kubernetes v1.22. #9205
*   \[CORRECCIÓN DE ERRORES] Corrija la carrera de datos en la carga de write-ahead-log (WAL). #9259

## 2.29.1 / 2021-08-11

*   \[BUGFIX] tsdb: alinear el int64 al que se accede atómicamente para evitar el pánico en 32 bits
    arcos. #9192

## 2.29.0 / 2021-08-11

Nota para usuarios de macOS: Debido a [cambios en el próximo Go 1.17](https://tip.golang.org/doc/go1.17#darwin),
esta es la última versión de Prometheus compatible con macOS 10.12 Sierra.

*   \[CAMBIAR] Promover `--storage.tsdb.allow-overlapping-blocks` bandera a estable. #9117
*   \[CAMBIAR] Promover `--storage.tsdb.retention.size` bandera a estable. #9004
*   \[CARACTERÍSTICA] Agregue la detección de servicios de Kuma. #8844
*   \[CARACTERÍSTICA] Agregar `present_over_time` Función PromQL. #9097
*   \[CARACTERÍSTICA] Permita configurar el almacenamiento ejemplar a través de un archivo y hágalo recargable. #8974
*   \[CARACTERÍSTICA] INTERFAZ DE USUARIO: Permite seleccionar el intervalo de tiempo con el arrastre del mouse. #8977
*   \[FEATURE] promtool: Agregar indicador de indicadores de características `--enable-feature`. #8958
*   \[FEATURE] promtool: Agregue file_sd validación de archivos. #8950
*   \[MEJORA] Reduzca el bloqueo de las solicitudes de escritura remota salientes de la recolección de elementos no utilizados en serie. #9109
*   \[MEJORA] Mejore el rendimiento de la decodificación de registros de escritura anticipada. #9106
*   \[MEJORA] Mejore el rendimiento de los apéndices en TSDB reduciendo el uso de mutexes. #9061
*   \[MEJORA] Permitir la configuración `max_samples_per_send` para metadatos de escritura remota. #8959
*   \[MEJORA] Agregar `__meta_gce_interface_ipv4_<name>` meta etiqueta para el descubrimiento de GCE. #8978
*   \[MEJORA] Agregar `__meta_ec2_availability_zone_id` metaetiqueta para el descubrimiento de EC2. #8896
*   \[MEJORA] Agregar `__meta_azure_machine_computer_name` metaetiqueta para la detección de Azure. #9112
*   \[MEJORA] Agregar `__meta_hetzner_hcloud_labelpresent_<labelname>` meta etiqueta al descubrimiento de Hetzner. #9028
*   \[MEJORA] promtool: Agregue eficiencia de compactación a `promtool tsdb analyze` Informes. #8940
*   \[MEJORA] promtool: Permite configurar la duración máxima del bloque para el relleno a través de `--max-block-duration` bandera. #8919
*   \[MEJORA] Interfaz de usuario: Agregue clasificación y filtrado a la página de banderas. #8988
*   \[MEJORA] Interfaz de usuario: mejora el rendimiento de la representación de páginas de alertas. #9005
*   \[CORRECCIÓN DE ERRORES] Registre cuando el tamaño total del símbolo supere los 2^32 bytes, lo que provoca un error en la compactación, y omita la compactación. #9104
*   \[CORRECCIÓN DE ERRORES] Corregir incorrecto `target_limit` recarga de valor cero. #9120
*   \[CORRECCIÓN DE ERRORES] Arreglar cabeza GC y pendiente de lectura de la condición de carrera. #9081
*   \[CORRECCIÓN DE ERRORES] Corrija el manejo de la marca de tiempo en el analizador OpenMetrics. #9008
*   \[CORRECCIÓN DE ERRORES] Corregir posibles métricas duplicadas en `/federate` punto final al especificar varios emparejadores. #8885
*   \[CORRECCIÓN DE ERRORES] Corregir la configuración del servidor y la validación para la autenticación a través del certificado de cliente. #9123
*   \[CORRECCIÓN DE ERRORES] Conceder `start` y `end` de nuevo como nombres de etiqueta en consultas PromQL. No se permitieron desde la introducción de la función @ timestamp. #9119

## 2.28.1 / 2021-07-01

*   \[CORRECCIÓN DE ERRORES]: HTTP SD: Permitir `charset` especificación en `Content-Type` encabezado. #8981
*   \[BUGFIX]: HTTP SD: Manejo de correcciones de grupos objetivo desaparecidos. #9019
*   \[CORRECCIÓN DE ERRORES]: Corrija el manejo incorrecto a nivel de registro después de pasar a go-kit / log. #9021

## 2.28.0 / 2021-06-21

*   \[CAMBIAR] INTERFAZ DE USUARIO: Haga que el nuevo editor experimental de PromQL sea el predeterminado. #8925
*   \[CARACTERÍSTICA] Linode SD: Agregue el descubrimiento de servicios de Linode. #8846
*   \[CARACTERÍSTICA] HTTP SD: agregue la detección genérica de servicios basada en HTTP. #8839
*   \[CARACTERÍSTICA] Kubernetes SD: Permite configurar el acceso al servidor API a través de un archivo kubeconfig. #8811
*   \[CARACTERÍSTICA] INTERFAZ DE USUARIO: Agregue compatibilidad de visualización ejemplar a la interfaz de gráficos. #8832 #8945 #8929
*   \[CARACTERÍSTICA] Consul SD: Agregue compatibilidad con el espacio de nombres para Consul Enterprise. #8900
*   \[MEJORA] Promtool: Permite silenciar la salida al importar / rellenar datos. #8917
*   \[MEJORA] Consul SD: Admite la lectura de tokens desde el archivo. #8926
*   \[MEJORA] Reglas: Agregar un nuevo `.ExternalURL` variable de plantilla de campo de alerta, que contiene la URL externa del servidor Prometheus. #8878
*   \[MEJORA] Raspado: Añadir experimental `body_size_limit` ajuste de configuración de raspado para limitar el tamaño del cuerpo de respuesta permitido para los raspados de destino. #8833 #8886
*   \[MEJORA] Kubernetes SD: Agregue la etiqueta de nombre de clase de entrada para la detección de entrada. #8916
*   \[MEJORA] Interfaz de usuario: Muestra una pantalla de inicio con barra de progreso cuando el TSDB aún no está listo. #8662 #8908 #8909 #8946
*   \[MEJORA] SD: Agregar un contador de errores de creación de destino `prometheus_target_sync_failed_total` y mejorar el manejo de fallas de creación de objetivos. #8786
*   \[MEJORA] TSDB: Mejorar la validación de la longitud ejemplar del conjunto de etiquetas. #8816
*   \[MEJORA] TSDB: Agregar un `prometheus_tsdb_clean_start` que indica si un archivo de bloqueo TSDB de una ejecución anterior todavía existía al iniciarse. #8824
*   \[CORRECCIÓN DE ERRORES] Interfaz de usuario: En el editor experimental de PromQL, corrija el autocompletado y el análisis de valores flotantes especiales y mejore la obtención de metadatos de serie. #8856
*   \[CORRECCIÓN DE ERRORES] TSDB: Al fusionar fragmentos, divida los trozos resultantes si contienen más del máximo de 120 muestras. #8582
*   \[CORRECCIÓN DE ERRORES] SD: Arreglar el cálculo de la `prometheus_sd_discovered_targets` métrica cuando se utilizan varias detecciones de servicios. #8828

## 2.27.1 / 2021-05-18

Esta versión contiene una corrección de errores para un problema de seguridad en el punto de enlace de la API. Un
El atacante puede crear una dirección URL especial que redirija a un usuario a cualquier extremo a través de un
Respuesta HTTP 302. Ver el [aviso de seguridad][GHSA-vx57-7f4q-fpc7] para más detalles.

[GHSA-vx57-7f4q-fpc7]: https://github.com/prometheus/prometheus/security/advisories/GHSA-vx57-7f4q-fpc7

Esta vulnerabilidad ha sido reportada por Aaron Devaney de MDSec.

*   \[CORRECCIÓN DE ERRORES] SEGURIDAD: Corregir redireccionamientos arbitrarios en el extremo /new (CVE-2021-29622)

## 2.27.0 / 2021-05-12

*   \[CAMBIAR] Escritura remota: Métrica `prometheus_remote_storage_samples_bytes_total` renombrado a `prometheus_remote_storage_bytes_total`. #8296
*   \[CARACTERÍSTICA] Promtool: Funcionalidad de evaluación de reglas retroactivas. #7675
*   \[CARACTERÍSTICA] Configuración: Expansión de variables de entorno para etiquetas externas. Atrás `--enable-feature=expand-external-labels` bandera. #8649
*   \[CARACTERÍSTICA] TSDB: Agregar un indicador(`--storage.tsdb.max-block-chunk-segment-size`) para controlar el tamaño máximo de archivo de fragmentos de los bloques para instancias pequeñas de Prometheus. #8478
*   \[CARACTERÍSTICA] Interfaz de usuario: Agrega un tema oscuro. #8604
*   \[CARACTERÍSTICA] AWS Lightsail Discovery: Agregue AWS Lightsail Discovery. #8693
*   \[CARACTERÍSTICA] Docker Discovery: Agregue Docker Service Discovery. #8629
*   \[CARACTERÍSTICA] OAuth: Permita que OAuth 2.0 se utilice en cualquier lugar donde se utilice un cliente HTTP. #8761
*   \[CARACTERÍSTICA] Escritura remota: envíe ejemplos a través de escritura remota. Experimental y deshabilitado por defecto. #8296
*   \[MEJORA] Digital Ocean Discovery: Añadir `__meta_digitalocean_vpc` etiqueta. #8642
*   \[MEJORA] Descubrimiento de Scaleway: Leer el secreto de Scaleway de un archivo. #8643
*   \[MEJORA] Raspado: Agregue límites configurables para el tamaño y el recuento de etiquetas. #8777
*   \[MEJORA] Interfaz de usuario: Agregue pasos de rango de tiempo de 16w y 26w. #8656
*   \[MEJORA] Plantillas: Habilitar el análisis de cadenas en `humanize` Funciones. #8682
*   \[CORRECCIÓN DE ERRORES] Interfaz de usuario: proporcione errores en lugar de página en blanco en la página de estado de TSDB. #8654 #8659
*   \[CORRECCIÓN DE ERRORES] TSDB: No entre en pánico al escribir discos muy grandes para el WAL. #8790
*   \[CORRECCIÓN DE ERRORES] TSDB: Evite el pánico cuando se hace referencia a la memoria mmaped después de cerrar el archivo. #8723
*   \[CORRECCIÓN DE ERRORES] Scaleway Discovery: Corregir la desreferencia de puntero nulo. #8737
*   \[CORRECCIÓN DE ERRORES] Consul Discovery: el reinicio ya no es necesario después de la actualización de configuración sin destinos. #8766

## 2.26.0 / 2021-03-31

Prometheus ahora está construido y es compatible con Go 1.16 (# 8544). Esto revierte el patrón de liberación de memoria agregado en Go 1.12. Esto hace que las métricas comunes de uso de RSS muestren un número más preciso para la memoria real utilizada por Prometheus. Puedes leer más detalles [aquí](https://www.bwplotka.dev/2019/golang-memory-monitoring/).

Tenga en cuenta que desde esta versión Prometheus utiliza Alertmanager v2 de forma predeterminada.

*   \[CAMBIAR] Alertas: uso de la API de Alertmanager v2 de forma predeterminada. #8626
*   \[CAMBIAR] Prometheus/Promtool: Según lo acordado en la cumbre de desarrollo, los binarios ahora están imprimiendo ayuda y uso para stdout en lugar de stderr. #8542
*   \[CARACTERÍSTICA] Remoto: agregue compatibilidad con el método de autenticación AWS SigV4 para remote_write. #8509
*   \[CARACTERÍSTICA] Detección de Scaleway: Agregue la detección de servicios de Scaleway. #8555
*   \[CARACTERÍSTICA] PromQL: Permitir desplazamientos negativos. Atrás `--enable-feature=promql-negative-offset` bandera. #8487
*   \[CARACTERÍSTICA] **experimental** Ejemplos: Agregue almacenamiento en memoria para ejemplos. Atrás `--enable-feature=exemplar-storage` bandera. #6635
*   \[CARACTERÍSTICA] Interfaz de usuario: agregue autocompletado avanzado, resaltado de sintaxis y linting a la entrada de consulta de página de gráfico. #8634
*   \[MEJORA] Digital Ocean Discovery: Añadir `__meta_digitalocean_image` etiqueta. #8497
*   \[MEJORA] PromQL: Añadir `last_over_time`, `sgn`, `clamp` Funciones. #8457
*   \[MEJORA] Scrape: agregue compatibilidad para especificar el tipo de credenciales de encabezado de autorización con Bearer de forma predeterminada. #8512
*   \[MEJORA] Raspar: Añadir `follow_redirects` opción para raspar la configuración. #8546
*   \[MEJORA] Remoto: permite reintentos en el código de respuesta HTTP 429 para remote_write. Deshabilitado de forma predeterminada. Ver [documentos de configuración](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write) para más detalles. #8237 #8477
*   \[MEJORA] Remoto: permite configurar encabezados personalizados para remote_read. Ver [documentos de configuración](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_read) para más detalles. #8516
*   \[MEJORA] INTERFAZ DE USUARIO: Al presionar Entrar ahora se desencadena una nueva consulta. #8581
*   \[MEJORA] UI: Mejor manejo de reglas largas y nombres en el `/rules` y `/targets` Páginas. #8608 #8609
*   \[MEJORA] Interfaz de usuario: Agregar botón contraer/expandir todo en el botón `/targets` página. #8486
*   \[CORRECCIÓN DE ERRORES] TSDB: Eliminación ansiosa de bloques extraíbles en cada compactación, ahorrando el uso máximo de espacio en disco. #8007
*   \[CORRECCIÓN DE ERRORES] PromQL: Solucione el soporte del analizador para caracteres especiales como`炬`. #8517
*   \[CORRECCIÓN DE ERRORES] Reglas: Se produce un error al actualizar el estado de la regla para anexar/confirmar. #8619

## 2.25.2 / 2021-03-16

*   \[CORRECCIÓN DE ERRORES] Arregle la ingestión de rasguños cuando el reloj de pared cambie, por ejemplo, en suspensión. #8601

## 2.25.1 / 2021-03-14

*   \[CORRECCIÓN DE ERRORES] Corregir un bloqueo en `promtool` cuando se utiliza una subconsulta con resolución predeterminada. #8569
*   \[CORRECCIÓN DE ERRORES] Corrige un error que podía devolver puntos de datos duplicados en las consultas. #8591
*   \[CORRECCIÓN DE ERRORES] Solucione bloqueos con arm64 cuando se compila con go1.16. #8593

## 2.25.0 / 2021-02-17

Esta versión incluye una nueva `--enable-feature=` indicador que habilita
características experimentales. Dichas características podrían cambiarse o eliminarse en el futuro.

En la próxima versión menor (2.26), Prometheus utilizará la API de Alertmanager v2.
Se hará por defecto `alertmanager_config.api_version` Para `v2`.
Alertmanager API v2 se lanzó en Alertmanager v0.16.0 (lanzado en enero)
2019\).

*   \[CARACTERÍSTICA] **experimental** API: Acepte solicitudes de remote_write. Detrás del indicador --enable-feature=remote-write-receiver. #8424
*   \[CARACTERÍSTICA] **experimental** PromQL: Añadir `@ <timestamp>` modificador. Detrás del indicador --enable-feature=promql-at-modifier. #8121 #8436 #8425
*   \[MEJORA] Agregue la propiedad name opcional a testgroup para obtener una mejor salida de error de prueba. #8440
*   \[MEJORA] Agregue advertencias en el Panel React en la página Gráfico. #8427
*   \[MEJORA] TSDB: Aumente el número de cucharones para la métrica de duración de compactación. #8342
*   \[MEJORA] Remoto: permite pasar encabezados HTTP personalizados remote_write. #8416
*   \[MEJORA] Mixins: Configuración de grafana de alcance. #8332
*   \[MEJORA] Kubernetes SD: Agregue metadatos de etiquetas de punto final. #8273
*   \[MEJORA] Interfaz de usuario: Exponga el número total de pares de etiquetas en el encabezado en la página de estadísticas de TSDB. #8343
*   \[MEJORA] TSDB: Vuelva a cargar bloques cada minuto, para detectar nuevos bloques y aplicar la retención con más frecuencia. #8340
*   \[CORRECCIÓN DE ERRORES] API: Corrija la URL global cuando la dirección externa no tiene puerto. #8359
*   \[CORRECCIÓN DE ERRORES] Relleno: Corregir el manejo de mensajes de error. #8432
*   \[CORRECCIÓN DE ERRORES] Relleno: Corrija el error "agregar muestra: fuera de los límites" cuando las series abarcan un bloque completo. #8476
*   \[CORRECCIÓN DE ERRORES] Dejar de usar el indicador --alertmanager.timeout. #8407
*   \[CORRECCIÓN DE ERRORES] Mixins: Admite métricas de escritura remota renombradas en v2.23 en alertas. #8423
*   \[CORRECCIÓN DE ERRORES] Remoto: Corrija la recolección de basura de series caídas en escritura remota. #8387
*   \[CORRECCIÓN DE ERRORES] Remoto: Registre los errores de escritura remota recuperables como advertencias. #8412
*   \[CORRECCIÓN DE ERRORES] TSDB: Elimine los bloques temporales anteriores a la versión 2.21 al inicio. #8353.
*   \[CORRECCIÓN DE ERRORES] INTERFAZ DE USUARIO: Corrija las claves duplicadas en la página /targets. #8456
*   \[CORRECCIÓN DE ERRORES] INTERFAZ DE USUARIO: Corrija la fuga del nombre de la etiqueta en el nombre de la clase. #8459

## 2.24.1 / 2021-01-20

*   \[MEJORA] Almacenar en caché los resultados de la autenticación básica para mejorar significativamente el rendimiento de los puntos finales HTTP (a través de una actualización de prometheus/exporter-toolkit).
*   \[CORRECCIÓN DE ERRORES] Evite la enumeración de usuarios cronometrando las solicitudes enviadas a los puntos de conexión HTTP autenticados (a través de una actualización de prometheus/exporter-toolkit).

## 2.24.0 / 2021-01-06

*   \[CARACTERÍSTICA] Agregue TLS y autenticación básica a los extremos HTTP. #8316
*   \[CARACTERÍSTICA] promtool: Añadir `check web-config` subcomando para comprobar los archivos de configuración web. #8319
*   \[CARACTERÍSTICA] promtool: Añadir `tsdb create-blocks-from openmetrics` subcomando para rellenar datos de métricas de un archivo OpenMetrics. #8084
*   \[MEJORA] API HTTP: consultas de error rápido con solo emparejadores vacíos. #8288
*   \[MEJORA] API HTTP: Admite emparejadores para la API de etiquetas. #8301
*   \[MEJORA] promtool: Mejora la comprobación de las URL pasadas en la línea de comandos. #7956
*   \[MEJORA] SD: Exponer IPv6 como una etiqueta en EC2 SD. #7086
*   \[MEJORA] SD: Reutilizar el cliente EC2, reduciendo la frecuencia de solicitud de credenciales. #8311
*   \[MEJORA] TSDB: Agregue registro cuando la compactación requiera más que el rango de tiempo del bloque. #8151
*   \[MEJORA] TSDB: Evite ejecuciones innecesarias de GC después de la compactación. #8276
*   \[CORRECCIÓN DE ERRORES] API HTTP: Evite el doble cierre del canal cuando salga varias veces a través de HTTP. #8242
*   \[CORRECCIÓN DE ERRORES] SD: Ignorar los registros CNAME en DNS SD para evitar espurios `Invalid SRV record` Advertencias. #8216
*   \[CORRECCIÓN DE ERRORES] SD: Evite el error de configuración desencadenado por selectores de etiquetas válidos en Kubernetes SD. #8285

## 2.23.0 / 2020-11-26

*   \[CAMBIAR] INTERFAZ DE USUARIO: Haga que la interfaz de usuario de React sea predeterminada. #8142
*   \[CAMBIAR] Escritura remota: las siguientes métricas se eliminaron o cambiaron de nombre en la escritura remota. #6815
    *   `prometheus_remote_storage_succeeded_samples_total` se eliminó y `prometheus_remote_storage_samples_total` se introdujo para todas las muestras intentadas de enviar.
    *   `prometheus_remote_storage_sent_bytes_total` se eliminó y se reemplazó con `prometheus_remote_storage_samples_bytes_total` y `prometheus_remote_storage_metadata_bytes_total`.
    *   `prometheus_remote_storage_failed_samples_total` -> `prometheus_remote_storage_samples_failed_total` .
    *   `prometheus_remote_storage_retried_samples_total` -> `prometheus_remote_storage_samples_retried_total`.
    *   `prometheus_remote_storage_dropped_samples_total` -> `prometheus_remote_storage_samples_dropped_total`.
    *   `prometheus_remote_storage_pending_samples` -> `prometheus_remote_storage_samples_pending`.
*   \[CAMBIAR] Remoto: no recopile métricas de marca de tiempo no inicializadas. #8060
*   \[CARACTERÍSTICA] \[EXPERIMENTAL] Escritura remota: permite que los metadatos de la métrica se propaguen a través de la escritura remota. Se introdujeron las siguientes nuevas métricas: `prometheus_remote_storage_metadata_total`, `prometheus_remote_storage_metadata_failed_total`, `prometheus_remote_storage_metadata_retried_total`, `prometheus_remote_storage_metadata_bytes_total`. #6815
*   \[MEJORA] Escritura remota: se ha agregado una métrica `prometheus_remote_storage_max_samples_per_send` para escritura remota. #8102
*   \[MEJORA] TSDB: Haga que el nombre del directorio de la instantánea tenga siempre la misma longitud. #8138
*   \[MEJORA] TSDB: Cree un punto de control solo una vez al final de todas las compactaciones de la cabeza. #8067
*   \[MEJORA] TSDB: Evite que la API de series golpee los trozos. #8050
*   \[MEJORA] TSDB: Nombre de la etiqueta de caché y último valor al agregar series durante las compactaciones, lo que hace que las compactaciones sean más rápidas. #8192
*   \[MEJORA] PromQL: Rendimiento mejorado del método Hash que hace que las consultas sean un poco más rápidas. #8025
*   \[MEJORA] promtool: `tsdb list` ahora imprime tamaños de bloque. #7993
*   \[MEJORA] promtool: Calcule menta y máximo por prueba evitando cálculos innecesarios. #8096
*   \[MEJORA] SD: Agregue filtrado de servicios a Docker Swarm SD. #8074
*   \[CORRECCIÓN DE ERRORES] React UI: Arregle la pantalla del botón cuando no haya paneles. #8155
*   \[CORRECCIÓN DE ERRORES] PromQL: Método Fix timestamp() para el selector de vectores entre paréntesis. #8164
*   \[CORRECCIÓN DE ERRORES] PromQL: No incluyas expresiones representadas en errores de análisis de PromQL. #8177
*   \[BUGFIX] web: Soluciona el pánico con doble cierre() de canal en la llamada `/-/quit/`. #8166
*   \[CORRECCIÓN DE ERRORES] TSDB: Se corrigió la corrupción de WAL en escrituras parciales dentro de una página que causaba `invalid checksum` error en la reproducción de WAL. #8125
*   \[CORRECCIÓN DE ERRORES] Actualizar métricas de configuración `prometheus_config_last_reload_successful` y `prometheus_config_last_reload_success_timestamp_seconds` justo después de la validación inicial antes de iniciar TSDB.
*   \[BUGFIX] promtool: Detecta correctamente nombres de etiquetas duplicados en la exposición.

## 2.22.2 / 2020-11-16

*   \[CORRECCIÓN DE ERRORES] Arreglar la condición de la carrera en los rascadores de sincronización/detención/recarga. #8176

## 2.22.1 / 2020-11-03

*   \[CORRECCIÓN DE ERRORES] Corrija los posibles errores "mmap: argumento no válido" al cargar los trozos de cabeza, después de un apagado sucio, realizando reparaciones de lectura. #8061
*   \[CORRECCIÓN DE ERRORES] Corrija las métricas de servicio y la API al volver a cargar la configuración de scrape. #8104
*   \[CORRECCIÓN DE ERRORES] Corrija el cálculo del tamaño del fragmento de la cabeza para la retención basada en el tamaño. #8139

## 2.22.0 / 2020-10-07

Como se anunció en las notas de la versión 2.21.0, la API experimental de gRPC v2 ha sido
Quitado.

*   \[CAMBIAR] web: Eliminar APIv2. #7935
*   \[MEJORA] Interfaz de usuario de React: Implemente la sección de estadísticas del cabezal de TSDB que falta. #7876
*   \[MEJORA] Interfaz de usuario: Agregue el botón Contraer todo a la página de destinos. #6957
*   \[MEJORA] INTERFAZ DE USUARIO: Aclarar el estado de alerta alternar a través del icono de casilla de verificación. #7936
*   \[MEJORA] Agregar `rule_group_last_evaluation_samples` y `prometheus_tsdb_data_replay_duration_seconds` Métricas. #7737 #7977
*   \[MEJORA] Maneje con gracia tipos de registro WAL desconocidos. #8004
*   \[MEJORA] Emita una advertencia para sistemas de 64 bits que ejecutan binarios de 32 bits. #8012
*   \[CORRECCIÓN DE ERRORES] Ajuste las marcas de tiempo de raspado para alinearlas con el cronograma previsto, reduciendo efectivamente el tamaño del bloque. Solución alternativa para una regresión en go1.14+. #7976
*   \[BUGFIX] promtool: Asegúrese de que las reglas de alerta estén marcadas como restauradas en las pruebas unitarias. #7661
*   \[CORRECCIÓN DE ERRORES] Eureka: Corregir el descubrimiento de servicios cuando se compila en 32 bits. #7964
*   \[CORRECCIÓN DE ERRORES] No hagas una optimización literal de la coincidencia de regex cuando no distingas entre mayúsculas y minúsculas. #8013
*   \[CORRECCIÓN DE ERRORES] Corrija la interfaz de usuario clásica que a veces ejecuta consultas para consultas instantáneas cuando está en modo de consulta de rango. #7984

## 2.21.0 / 2020-09-11

Esta versión está construida con Go 1.15, que deja de usar [X.509 Nombre común](https://golang.org/doc/go1.15#commonname)
en validación de certificados TLS.

En el improbable caso de que utilice la API gRPC v2 (que está limitada a TSDB
admin commands), tenga en cuenta que eliminaremos esta API experimental en el
próxima versión menor 2.22.

*   \[CAMBIAR] Deshabilite HTTP/2 debido a problemas con el cliente Go HTTP/2. #7588 #7701
*   \[CAMBIAR] PromQL: `query_log_file` la ruta de acceso ahora es relativa al archivo de configuración. #7701
*   \[CAMBIAR] Promtool: Reemplace la herramienta de línea de comandos tsdb por un subcomando tsdb de promtool. #6088
*   \[CAMBIAR] Reglas: Etiqueta `rule_group_iterations` métrica con nombre de grupo. #7823
*   \[CARACTERÍSTICA] Eureka SD: Descubrimiento de nuevos servicios. #3369
*   \[CARACTERÍSTICA] Hetzner SD: Descubrimiento de nuevos servicios. #7822
*   \[CARACTERÍSTICA] Kubernetes SD: Soporta Kubernetes EndpointSlices. #6838
*   \[CARACTERÍSTICA] Scrape: Añadir límite de destinos por scrape-config. #7554
*   \[MEJORA] Admite duraciones compuestas en PromQL, configuración y ui, por ejemplo, 1h30m. #7713 #7833
*   \[MEJORA] DNS SD: Agregue metaetiquetas de puerto y destino de registro SRV. #7678
*   \[MEJORA] Docker Swarm SD: Admite tareas y servicio sin puertos publicados. #7686
*   \[MEJORA] PromQL: Reduzca la cantidad de datos consultados por la lectura remota cuando una subconsulta tiene un desplazamiento. #7667
*   \[MEJORA] Promtool: Añadir `--time` opción para consultar comandos instantáneos. #7829
*   \[MEJORA] UI: Respeta el `--web.page-title` en la interfaz de usuario de React. #7607
*   \[MEJORA] Interfaz de usuario: agregue duración, etiquetas y anotaciones a la página de alertas en la interfaz de usuario de React. #7605
*   \[MEJORA] INTERFAZ DE USUARIO: Agregue la duración en la página reglas de la interfaz de usuario de React, oculte las anotaciones y las etiquetas si están vacías. #7606
*   \[CORRECCIÓN DE ERRORES] API: Serie de deduplicados en /api/v1/series. #7862
*   \[CORRECCIÓN DE ERRORES] PromQL: Elimine el nombre de la métrica en la comparación bool entre dos vectores instantáneos. #7819
*   \[CORRECCIÓN DE ERRORES] PromQL: Salga con un error cuando no se pueden analizar los parámetros de tiempo. #7505
*   \[CORRECCIÓN DE ERRORES] Lectura remota: Vuelva a agregar el seguimiento eliminado accidentalmente para las solicitudes de lectura remota. #7916
*   \[CORRECCIÓN DE ERRORES] Reglas: detecta campos adicionales en archivos de reglas. #7767
*   \[CORRECCIÓN DE ERRORES] Reglas: No permitir la sobrescritura del nombre de la métrica en el `labels` sección de reglas de grabación. #7787
*   \[CORRECCIÓN DE ERRORES] Reglas: Mantenga la marca de tiempo de evaluación en todas las recargas. #7775
*   \[CORRECCIÓN DE ERRORES] Raspado: No detenga los rasguños en curso durante la recarga. #7752
*   \[CORRECCIÓN DE ERRORES] TSDB: Solución `chunks.HeadReadWriter: maxt of the files are not set` error. #7856
*   \[CORRECCIÓN DE ERRORES] TSDB: Elimine bloques atómicamente para evitar daños cuando haya un pánico / bloqueo durante la eliminación. #7772
*   \[CORRECCIÓN DE ERRORES] Triton SD: Arregla el pánico cuando triton_sd_config es nulo. #7671
*   \[CORRECCIÓN DE ERRORES] INTERFAZ DE USUARIO: Corrija el error de interfaz de usuario de reacción con series que se activan y desactivan. #7804
*   \[CORRECCIÓN DE ERRORES] INTERFAZ DE USUARIO: Corrige el error de estilo para las etiquetas de destino con nombres especiales en la interfaz de usuario de React. #7902
*   \[CORRECCIÓN DE ERRORES] Web: Detenga los servidores CMUX y GRPC incluso con conexiones obsoletas, evitando que el servidor se detenga en SIGTERM. #7810

## 2.20.1 / 2020-08-05

*   \[CORRECCIÓN DE ERRORES] SD: Reduzca el tiempo de espera del reloj Consul a 2 m y ajuste el tiempo de espera de la solicitud en consecuencia. #7724

## 2.20.0 / 2020-07-22

Esta versión cambia la compresión WAL de opt-in a default. La compresión WAL evitará una degradación a v2.10 o anterior sin eliminar el WAL. Deshabilite la compresión WAL explícitamente estableciendo el indicador de línea de comandos `--no-storage.tsdb.wal-compression` si necesita degradar a v2.10 o anterior.

*   \[CHANGE] promtool: Se ha cambiado la numeración de reglas de 0 a 1 al informar errores de reglas. #7495
*   \[CAMBIAR] Lectura remota: Añadida `prometheus_remote_storage_read_queries_total` contador y `prometheus_remote_storage_read_request_duration_seconds` histograma, eliminado `prometheus_remote_storage_remote_read_queries_total` mostrador.
*   \[CAMBIAR] Escritura remota: se agregaron buckets para duraciones más largas a `prometheus_remote_storage_sent_batch_duration_seconds` histograma.
*   \[CAMBIAR] TSDB: La compresión WAL está habilitada de forma predeterminada. #7410
*   \[CARACTERÍSTICA] PromQL: Añadido `group()` agregador. #7480
*   \[CARACTERÍSTICA] SD: Añadido Docker Swarm SD. #7420
*   \[CARACTERÍSTICA] SD: Añadido DigitalOcean SD. #7407
*   \[CARACTERÍSTICA] SD: Se agregó la opción de configuración de Openstack para consultar puntos finales alternativos. #7494
*   \[MEJORA] Configuración: Salga temprano del archivo de configuración no válido y señalizarlo con el código de salida 2. #7399
*   \[MEJORA] PromQL: `without` ahora es un identificador de métrica válido. #7533
*   \[MEJORA] PromQL: Coincidencia optimizada de etiquetas regex para literales dentro del patrón o como prefijo/sufijo. #7453 #7503
*   \[MEJORA] promtool: Se agregaron parámetros de rango de tiempo para la API de etiquetas en promtool. #7463
*   \[MEJORA] Escritura remota: incluya muestras en espera en el canal en la métrica de muestras pendientes. Registre el número de muestras eliminadas en el apagado duro. #7335
*   \[MEJORA] Scrape: Ingiera métricas de informe de raspado sintético atómicamente con las métricas raspadas correspondientes. #7562
*   \[MEJORA] SD: Reduzca los tiempos de espera para Openstack SD. #7507
*   \[MEJORA] SD: Utilice un tiempo de espera de 10 m para los relojes Consul. #7423
*   \[MEJORA] SD: Se ha añadido la metaetiqueta AMI para EC2 SD. #7386
*   \[MEJORA] TSDB: Incremente la métrica de corrupción de WAL también en la corrupción de WAL durante el punto de control. #7491
*   \[MEJORA] TSDB: Rendimiento de consulta mejorado para etiquetas de alta cardinalidad. #7448
*   \[MEJORA] Interfaz de usuario: Muestra fechas y marcas de tiempo en la página de estado. #7544
*   \[MEJORA] Interfaz de usuario: Se ha mejorado el desplazamiento al seguir enlaces de fragmentos hash. #7456
*   \[MEJORA] INTERFAZ DE USUARIO: React UI representa los números en alertas de una manera más legible para el ser humano. #7426
*   \[CORRECCIÓN DE ERRORES] API: Se ha corregido el código de estado de error en la API de consulta. #7435
*   \[CORRECCIÓN DE ERRORES] PromQL: Fijo `avg` y `avg_over_time` para los desbordamientos de NaN, Inf y float64. #7346
*   \[CORRECCIÓN DE ERRORES] PromQL: Se ha corregido el error off-by-one en `histogram_quantile`. #7393
*   \[BUGFIX] promtool: Admite duraciones prolongadas en las pruebas unitarias de reglas. #6297
*   \[CORRECCIÓN DE ERRORES] Scrape: Corregir el recuento insuficiente de `scrape_samples_post_metric_relabeling` en caso de errores. #7342
*   \[CORRECCIÓN DE ERRORES] TSDB: No entre en pánico por las corrupciones de WAL. #7550
*   \[CORRECCIÓN DE ERRORES] TSDB: Evite dejar archivos vacíos en `chunks_head`, causando errores de inicio. #7573
*   \[CORRECCIÓN DE ERRORES] TSDB: Se ha corregido la carrera entre compact (gc, populate) y head append causando un error de símbolo desconocido. #7560
*   \[CORRECCIÓN DE ERRORES] TSDB: Se ha corregido un error de símbolo desconocido durante la compactación de la cabeza. #7526
*   \[CORRECCIÓN DE ERRORES] TSDB: Se ha corregido el pánico durante el registro de métricas de TSDB. #7501
*   \[CORRECCIÓN DE ERRORES] TSDB: Fijo `--limit` indicador de línea de comandos en `tsdb` herramienta. #7430

## 2.19.3 / 2020-07-24

*   \[CORRECCIÓN DE ERRORES] TSDB: No entre en pánico por las corrupciones de WAL. #7550
*   \[CORRECCIÓN DE ERRORES] TSDB: Evite dejar archivos vacíos en chunks_head, causando fallas de inicio. #7573

## 2.19.2 / 2020-06-26

*   \[CORRECCIÓN DE ERRORES] Escritura remota: solucione el pánico al volver a cargar la configuración con parámetros de cola modificados. #7452

## 2.19.1 / 2020-06-18

*   \[CORRECCIÓN DE ERRORES] TSDB: Corrija el truncamiento del archivo m-map que conduce a archivos no secuenciales. #7414

## 2.19.0 / 2020-06-09

*   \[CARACTERÍSTICA] TSDB: Memoria-mapear trozos completos de bloque Head (en memoria) del disco. Esto reduce el espacio de memoria y hace que los reinicios sean más rápidos. #6679
*   \[MEJORA] Descubrimiento: Se ha agregado compatibilidad con el descubrimiento para las zonas globales de Triton. #7250
*   \[MEJORA] Mayor retraso de reenvío de alertas para ser más tolerantes a las fallas. #7228
*   \[MEJORA] Lectura remota: Añadido `prometheus_remote_storage_remote_read_queries_total` para contar el número total de consultas de lectura remota. #7328
*   \[ENHANCEMEMT] Se agregaron parámetros de intervalo de tiempo para los nombres de etiqueta y los valores de etiqueta API. #7288
*   \[MEJORA] TSDB: Contención reducida de forma aislada para cargas altas. #7332
*   \[CORRECCIÓN DE ERRORES] PromQL: Se eliminó la colisión al verificar si había etiquetas duplicadas. #7058
*   \[CORRECCIÓN DE ERRORES] Interfaz de usuario de React: No anule los datos al hacer clic en la pestaña actual. #7243
*   \[CORRECCIÓN DE ERRORES] PromQL: Realice un seguimiento correcto del número de ejemplos para una consulta. #7307
*   \[CORRECCIÓN DE ERRORES] PromQL: Devuelve NaN cuando los cubos de histograma tienen 0 observaciones. #7318

## 2.18.2 / 2020-06-09

*   \[CORRECCIÓN DE ERRORES] TSDB: Corregir resultados de consulta incorrectos al usar Prometheus con lecturas remotas configuradas #7361

## 2.18.1 / 2020-05-07

*   \[CORRECCIÓN DE ERRORES] TSDB: API de instantánea fija. #7217

## 2.18.0 / 2020-05-05

*   \[CAMBIAR] Federación: solo use TSDB local para la federación (ignore la lectura remota). #7096
*   \[CAMBIAR] Reglas: `rule_evaluations_total` y `rule_evaluation_failures_total` tener un `rule_group` etiqueta ahora. #7094
*   \[CARACTERÍSTICA] Rastreo: Se agregó soporte experimental de Jaeger #7148
*   \[MEJORA] TSDB: Reduzca significativamente el tamaño de WAL mantenido después de un corte de bloque. #7098
*   \[MEJORA] Descubrimiento: Agregar `architecture` metaetiqueta para EC2. #7000
*   \[CORRECCIÓN DE ERRORES] UI: Se ha corregido el MinTime incorrecto informado por /status. #7182
*   \[CORRECCIÓN DE ERRORES] React UI: Se ha corregido la leyenda multiseleccional en OSX. #6880
*   \[CORRECCIÓN DE ERRORES] Escritura remota: Se ha corregido el caso de borde de resharding bloqueado. #7122
*   \[CORRECCIÓN DE ERRORES] Escritura remota: Se ha corregido el cambio de escritura remota que no se actualiza en el cambio de configuración de reetiquetado. #7073

## 2.17.2 / 2020-04-20

*   \[CORRECCIÓN DE ERRORES] Federación: Registrar métricas de federación #7081
*   \[CORRECCIÓN DE ERRORES] PromQL: Corregir el pánico en el manejo de errores del analizador #7132
*   \[CORRECCIÓN DE ERRORES] Reglas: Corregir las recargas que se bloquean al eliminar un grupo de reglas que se está evaluando #7138
*   \[CORRECCIÓN DE ERRORES] TSDB: Corregir una pérdida de memoria cuando prometheus comienza con un TSDB WAL #7135 vacío
*   \[CORRECCIÓN DE ERRORES] TSDB: Hacer que el aislamiento sea más robusto para los pánicos en los controladores web #7129 #7136

## 2.17.1 / 2020-03-26

*   \[CORRECCIÓN DE ERRORES] TSDB: Corregir la regresión del rendimiento de las consultas que aumentaba el uso de memoria y CPU #7051

## 2.17.0 / 2020-03-24

Esta versión implementa el aislamiento en TSDB. Las consultas de API y las reglas de grabación son
garantizado para ver solo rasguños completos y reglas de grabación completas. Esto viene con un
cierta sobrecarga en el uso de recursos. Dependiendo de la situación, puede haber
algún aumento en el uso de memoria, el uso de CPU o la latencia de consulta.

*   \[CARACTERÍSTICA] TSDB: Aislamiento de soporte #6841
*   \[MEJORA] PromQL: Permitir más palabras clave como nombres de métricas #6933
*   \[MEJORA] Interfaz de usuario de React: Agregar normalización de urls de host local en la página de destinos #6794
*   \[MEJORA] Lectura remota: Lectura simultánea desde el almacenamiento remoto #6770
*   \[MEJORA] Reglas: Marque la serie de reglas eliminadas como obsoleta después de una recarga #6745
*   \[MEJORA] Scrape: Log scrape anexar errores como depuración en lugar de advertir #6852
*   \[MEJORA] TSDB: Mejore el rendimiento de las consultas para consultas que golpean parcialmente la cabeza #6676
*   \[MEJORA] Consul SD: Exponer el estado del servicio como metaetiqueta #5313
*   \[MEJORA] EC2 SD: Exponer el ciclo de vida de la instancia EC2 como metaetiqueta #6914
*   \[MEJORA] Kubernetes SD: Exponer el tipo de servicio como metaetiqueta para el rol de servicio K8s #6684
*   \[MEJORA] Kubernetes SD: Exponer label_selector y field_selector #6807
*   \[MEJORA] Openstack SD: Exponer el identificador del hipervisor como metaetiqueta #6962
*   \[CORRECCIÓN DE ERRORES] PromQL: No escape de caracteres similares a HTML en el registro de consultas #6834 #6795
*   \[CORRECCIÓN DE ERRORES] React UI: Corregir los valores de la matriz de la tabla de datos #6896
*   \[CORRECCIÓN DE ERRORES] Interfaz de usuario de React: Corregir la nueva página de destinos que no se carga cuando se usan caracteres que no son ASCII #6892
*   \[CORRECCIÓN DE ERRORES] Lectura remota: Corregir la duplicación de métricas leídas desde el almacenamiento remoto con etiquetas externas #6967 #7018
*   \[CORRECCIÓN DE ERRORES] Escritura remota: Registre las métricas de wal watcher y live reader para todos los controles remotos, no solo para el primero #6998
*   \[CORRECCIÓN DE ERRORES] Scrape: Evite la eliminación de nombres de métricas al volver a etiquetar #6891
*   \[CORRECCIÓN DE ERRORES] Raspado: Corregir 'respuesta superflua. Errores de llamada writeHeader cuando el raspado falla bajo algunas circunstancias #6986
*   \[CORRECCIÓN DE ERRORES] Scrape: Corrige el bloqueo cuando las recargas están separadas por dos intervalos de scrape #7011

## 2.16.0 / 2020-02-13

*   \[CARACTERÍSTICA] React UI: Admite zona horaria local en /graph #6692
*   \[CARACTERÍSTICA] PromQL: agregar absent_over_time función de consulta #6490
*   \[CARACTERÍSTICA] Agregar registro opcional de consultas a su propio archivo #6520
*   \[MEJORA] Interfaz de usuario de React: Agregar compatibilidad con la página de reglas y la duración de "Xs ago" muestra #6503
*   \[MEJORA] Interfaz de usuario de React: página de alertas, reemplace las pestañas de los conmutadores de filtrado con casillas de verificación #6543
*   \[MEJORA] TSDB: Exportar métrica para errores de escritura WAL #6647
*   \[MEJORA] TSDB: mejore el rendimiento de las consultas para consultas que solo tocan las 2 horas de datos más recientes. #6651
*   \[MEJORA] PromQL: Refactorización en errores del analizador para mejorar los mensajes de error #6634
*   \[MEJORA] PromQL: Admite comas finales en la agrupación opta por #6480
*   \[MEJORA] Scrape: Reduzca el uso de memoria en las recargas reutilizando la caché de scrape #6670
*   \[MEJORA] Scrape: Agregar métricas para realizar un seguimiento de bytes y entradas en la caché de metadatos #6675
*   \[MEJORA] promtool: Agregue soporte para números de columna de línea para la salida de reglas no válidas #6533
*   \[MEJORA] Evite reiniciar grupos de reglas cuando sea innecesario #6450
*   \[CORRECCIÓN DE ERRORES] React UI: Enviar cookies en fetch() en navegadores más antiguos #6553
*   \[CORRECCIÓN DE ERRORES] React UI: adopte grafana flot fix para gráficos apilados #6603
*   \[BUFGIX] React UI: historial del navegador de páginas de gráficos rotos para que el botón Atrás funcione como se esperaba #6659
*   \[CORRECCIÓN DE ERRORES] TSDB: asegúrese de que la métrica compactionsSkipped esté registrada y registre el error adecuado si se devuelve uno desde la cabeza. Init #6616
*   \[CORRECCIÓN DE ERRORES] TSDB: devuelve un error al ingerir series con etiquetas duplicadas #6664
*   \[CORRECCIÓN DE ERRORES] PromQL: Fijar precedencia de operador unario #6579
*   \[CORRECCIÓN DE ERRORES] PromQL: Respete query.timeout incluso cuando lleguemos a query.max-concurrency #6712
*   \[CORRECCIÓN DE ERRORES] PromQL: Corregir el manejo de cadenas y paréntesis en el motor, lo que afectó a React UI #6612
*   \[CORRECCIÓN DE ERRORES] PromQL: Elimine las etiquetas de salida devueltas por absent() si son producidas por varios emparejadores de etiquetas idénticos #6493
*   \[CORRECCIÓN DE ERRORES] Scrape: Validar que la entrada de OpenMetrics termina con `# EOF` #6505
*   \[CORRECCIÓN DE ERRORES] Lectura remota: devuelva el error correcto si las configuraciones no se pueden calcular como referencia a JSON #6622
*   \[CORRECCIÓN DE ERRORES] Escritura remota: Crear cliente remoto `Store` utilice el contexto pasado, que puede afectar al tiempo de apagado #6673
*   \[CORRECCIÓN DE ERRORES] Escritura remota: Mejore el cálculo de fragmentación en los casos en los que siempre estaríamos consistentemente atrasados mediante el seguimiento de muestras pendientes #6511
*   \[CORRECCIÓN DE ERRORES] Asegúrese de prometheus_rule_group métricas se eliminen cuando se elimine un grupo de reglas #6693

## 2.15.2 / 2020-01-06

*   \[CORRECCIÓN DE ERRORES] TSDB: Soporte fijo para bloques TSDB construidos con Prometheus antes de 2.1.0. #6564
*   \[CORRECCIÓN DE ERRORES] TSDB: Se han corregido los problemas de compactación de bloques en Windows. #6547

## 2.15.1 / 2019-12-25

*   \[CORRECCIÓN DE ERRORES] TSDB: Se ha corregido la raza en consultas simultáneas con los mismos datos. #6512

## 2.15.0 / 2019-12-23

*   \[CAMBIAR] Descubrimiento: Eliminado `prometheus_sd_kubernetes_cache_*` Métricas. Adicionalmente `prometheus_sd_kubernetes_workqueue_latency_seconds` y `prometheus_sd_kubernetes_workqueue_work_duration_seconds` las métricas ahora muestran los valores correctos en segundos. #6393
*   \[CAMBIAR] Escritura remota: Cambiado `query` etiqueta en `prometheus_remote_storage_*` métricas para `remote_name` y `url`. #6043
*   \[CARACTERÍSTICA] API: Se ha agregado un nuevo punto de enlace para exponer metadatos por métrica `/metadata`. #6420 #6442
*   \[MEJORA] TSDB: Reducción significativa de la huella de memoria de los bloques TSDB cargados. #6418 #6461
*   \[MEJORA] TSDB: Optimizamos significativamente lo que almacenamos en búfer durante la compactación, lo que debería resultar en una menor huella de memoria durante la compactación. #6422 #6452 #6468 #6475
*   \[MEJORA] TSDB: Mejora la latencia de reproducción. #6230
*   \[MEJORA] TSDB: El tamaño WAL ahora se utiliza para el cálculo de retención basado en el tamaño. #5886
*   \[MEJORA] Lectura remota: se agregaron sugerencias de agrupación de consultas y rango a la solicitud de lectura remota #6401
*   \[MEJORA] Escritura remota: Agregada `prometheus_remote_storage_sent_bytes_total` contador por cola. #6344
*   \[MEJORA] promql: Rendimiento mejorado del analizador PromQL. #6356
*   \[MEJORA] React UI: Páginas faltantes implementadas como `/targets` #6276, página de estado de TSDB #6281 #6267 y muchas otras correcciones y mejoras de rendimiento.
*   \[MEJORA] promql: Prometheus ahora acepta espacios entre el rango de tiempo y el corchete cuadrado. p. ej. `[ 5m]` #6065
*   \[CORRECCIÓN DE ERRORES] Configuración: Se ha corregido la configuración de alertmanager para no perder objetivos cuando las configuraciones son similares. #6455
*   \[CORRECCIÓN DE ERRORES] Escritura remota: Valor de `prometheus_remote_storage_shards_desired` gauge muestra el valor bruto de los fragmentos deseados y se actualiza correctamente. #6378
*   \[CORRECCIÓN DE ERRORES] Reglas: Prometheus ahora falla en la evaluación de reglas y alertas donde los resultados de la métrica chocan con las etiquetas especificadas en `labels` campo. #6469
*   \[CORRECCIÓN DE ERRORES] API: API de metadatos de destino `/targets/metadata` ahora acepta vacío `match_targets` parámetro como en la especificación. #6303

## 2.14.0 / 2019-11-11

*   \[SEGURIDAD/CORRECCIÓN DE ERRORES] Interfaz de usuario: asegúrese de que se escapen las advertencias de la API. #6279
*   \[CARACTERÍSTICA] API: `/api/v1/status/runtimeinfo` y `/api/v1/status/buildinfo` extremos agregados para su uso por la interfaz de usuario de React. #6243
*   \[CARACTERÍSTICA] Interfaz de usuario de React: implemente la nueva interfaz de usuario experimental basada en React. #5694 y muchos más
    *   Se puede encontrar en `/new`.
    *   Todavía no todas las páginas están implementadas.
*   \[CARACTERÍSTICA] Estado: Estadísticas de cardinalidad agregadas a la página Runtime & Build Information. #6125
*   \[MEJORA/CORRECCIÓN DE ERRORES] Escritura remota: corrige los retrasos en la escritura remota después de una compactación. #6021
*   \[MEJORA] INTERFAZ DE USUARIO: Las alertas se pueden filtrar por estado. #5758
*   \[CORRECCIÓN DE ERRORES] API: los puntos de enlace del ciclo de vida devuelven 403 cuando no están habilitados. #6057
*   \[CORRECCIÓN DE ERRORES] Compilación: arreglar la compilación de Solaris. #6149
*   \[CORRECCIÓN DE ERRORES] Promtool: Elimine las advertencias de reglas duplicadas falsas al comprobar los archivos de reglas con alertas. #6270
*   \[CORRECCIÓN DE ERRORES] Escritura remota: restaure el uso del registrador de deduplicación en la escritura remota. #6113
*   \[CORRECCIÓN DE ERRORES] Escritura remota: no vuelva a particionar cuando no pueda enviar muestras. #6111
*   \[CORRECCIÓN DE ERRORES] Detección de servicios: los errores ya no se registran en la cancelación de contexto. #6116, #6133
*   \[CORRECCIÓN DE ERRORES] INTERFAZ DE USUARIO: maneja correctamente la respuesta nula de la API. #6071

## 2.13.1 / 2019-10-16

*   \[CORRECCIÓN DE ERRORES] Arreglar el pánico en las compilaciones ARM de Prometheus. #6110
*   \[BUGFIX] promql: solucione el pánico potencial en el registrador de consultas. #6094
*   \[CORRECCIÓN DE ERRORES] Múltiples errores de http: respuesta superflua. Llamada WriteHeader en los registros. #6145

## 2.13.0 / 2019-10-04

*   \[SEGURIDAD/CORRECCIÓN DE ERRORES] INTERFAZ DE USUARIO: Corregir una vulnerabilidad de DOM XSS almacenada con el historial de consultas [CVE-2019-10215 (en inglés)](http://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2019-10215). #6098
*   \[CAMBIAR] Métricas: renombrado prometheus_sd_configs_failed_total a prometheus_sd_failed_configs y cambiado a Gauge #5254
*   \[MEJORA] Incluya la herramienta tsdb en las compilaciones. #6089
*   \[MEJORA] Detección de servicios: agregue nuevos tipos de direcciones de nodo para kubernetes. #5902
*   \[MEJORA] INTERFAZ DE USUARIO: muestra advertencias si la consulta ha devuelto algunas advertencias. #5964
*   \[MEJORA] Escritura remota: reduzca el uso de memoria de la caché de la serie. #5849
*   \[MEJORA] Lectura remota: utilice la transmisión de lectura remota para reducir el uso de memoria. #5703
*   \[MEJORA] Métricas: se agregaron métricas para la escritura remota de fragmentos máximos / mínimos / deseados al administrador de colas. #5787
*   \[MEJORA] Promtool: muestra las advertencias durante la consulta de etiquetas. #5924
*   \[MEJORA] Promtool: mejora los mensajes de error al analizar reglas incorrectas. #5965
*   \[MEJORA] Promtool: más reglas promlint. #5515
*   \[CORRECCIÓN DE ERRORES] Promtool: corrige la inconsistencia de grabación debido a etiquetas duplicadas. #6026
*   \[CORRECCIÓN DE ERRORES] Interfaz de usuario: corrige la vista de detección de servicios cuando se accede a ella desde destinos incorrectos. #5915
*   \[CORRECCIÓN DE ERRORES] Formato de métricas: el analizador OpenMetrics se bloquea en una entrada corta. #5939
*   \[CORRECCIÓN DE ERRORES] UI: evite los valores truncados del eje Y. #6014

## 2.12.0 / 2019-08-17

*   \[CARACTERÍSTICA] Realice un seguimiento de las consultas PromQL actualmente activas en un archivo de registro. #5794
*   \[CARACTERÍSTICA] Habilitar y proporcionar binarios para `mips64` / `mips64le` Arquitecturas. #5792
*   \[MEJORA] Mejore la capacidad de respuesta de la interfaz de usuario web de destino y el punto de enlace de la API. #5740
*   \[MEJORA] Mejore el cálculo de fragmentos deseados de escritura remota. #5763
*   \[MEJORA] Vacíe las páginas TSDB con mayor precisión. tsdb#660
*   \[MEJORA] Agregar `prometheus_tsdb_retention_limit_bytes` métrico. tsdb#667
*   \[MEJORA] Agregue el registro durante la reproducción de TSDB WAL en el inicio. tsdb#662
*   \[MEJORA] Mejore el uso de la memoria TSDB. tsdb#653, tsdb#643, tsdb#654, tsdb#642, tsdb#627
*   \[CORRECCIÓN DE ERRORES] Compruebe si hay nombres de etiquetas duplicados en la lectura remota. #5829
*   \[CORRECCIÓN DE ERRORES] Marque la serie de reglas eliminadas como obsoleta en la próxima evaluación. #5759
*   \[CORRECCIÓN DE ERRORES] Corrija el error de JavaScript al mostrar una advertencia sobre la hora del servidor fuera de sincronización. #5833
*   \[CORRECCIÓN DE ERRORES] Arreglar `promtool test rules` pánico al proporcionar vacío `exp_labels`. #5774
*   \[CORRECCIÓN DE ERRORES] Solo verifique el último directorio cuando descubra el número de punto de control. #5756
*   \[CORRECCIÓN DE ERRORES] Corregir la propagación de errores en las funciones auxiliares del observador WAL. #5741
*   \[CORRECCIÓN DE ERRORES] Manejar correctamente las etiquetas vacías de las plantillas de alerta. #5845

## 2.11.2 / 2019-08-14

*   \[CORRECCIÓN DE ERRORES/SEGURIDAD] Corregir una vulnerabilidad de DOM XSS almacenada con el historial de consultas. #5888

## 2.11.1 / 2019-07-10

*   \[CORRECCIÓN DE ERRORES] Solucione el pánico potencial cuando prometeo está observando múltiples caminos de cuidadores de zoológicos. #5749

## 2.11.0 / 2019-07-09

*   \[CAMBIAR] Eliminar `max_retries` de queue_config (no se ha utilizado desde que se reescribió la escritura remota para utilizar el registro de escritura anticipada). #5649
*   \[CAMBIAR] El archivo meta `BlockStats` ya no contiene información de tamaño. Esto ahora se calcula dinámicamente y se mantiene en la memoria. También incluye el tamaño del meta archivo que no se incluía antes. tsdb#637
*   \[CAMBIAR] Métrica renombrada de `prometheus_tsdb_wal_reader_corruption_errors` Para `prometheus_tsdb_wal_reader_corruption_errors_total`. tsdb#622
*   \[CARACTERÍSTICA] Agregue la opción para usar Alertmanager API v2. #5482
*   \[CARACTERÍSTICA] Añadido `humanizePercentage` función para plantillas. #5670
*   \[CARACTERÍSTICA] Incluya InitContainers en Kubernetes Service Discovery. #5598
*   \[CARACTERÍSTICA] Proporcione la opción de comprimir registros WAL utilizando Snappy. [#609](https://github.com/prometheus/tsdb/pull/609)
*   \[MEJORA] Cree un nuevo segmento limpio al iniciar el WAL. tsdb#608
*   \[MEJORA] Reducir las asignaciones en las agregaciones de PromQL. #5641
*   \[MEJORA] Agregue advertencias de almacenamiento a los resultados de la API LabelValues y LabelNames. #5673
*   \[MEJORA] Agregar `prometheus_http_requests_total` métrico. #5640
*   \[MEJORA] Habilite la compilación openbsd/arm. #5696
*   \[MEJORA] Mejoras en la asignación de escritura remota. #5614
*   \[MEJORA] Mejora del rendimiento de las consultas: iteración y búsqueda eficientes en HashForLabels y HashWithoutLabels. #5707
*   \[MEJORA] Permitir la inyección de encabezados arbitrarios en promtool. #4389
*   \[MEJORA] Permitir el paso `external_labels` en grupos de pruebas unitarias de alerta. #5608
*   \[MEJORA] Permite globs para reglas cuando se realizan pruebas unitarias. #5595
*   \[MEJORA] Mejora de la coincidencia de intersecciones de postes. tsdb#616
*   \[MEJORA] Uso reducido de discos para WAL para configuraciones pequeñas. tsdb#605
*   \[MEJORA] Optimice las consultas mediante regexp para configurar búsquedas. tsdb#602
*   \[BUGFIX] resuelve la condición de la carrera en maxGauge. #5647
*   \[CORRECCIÓN DE ERRORES] Arreglar la fuga de conexión de ZooKeeper. #5675
*   \[CORRECCIÓN DE ERRORES] Mejora de la atomicidad de .tmp el reemplazo del bloque durante la compactación para el caso habitual. tsdb#636
*   \[CORRECCIÓN DE ERRORES] Corrija las "referencias de series desconocidas" después de un apagado limpio. tsdb#623
*   \[CORRECCIÓN DE ERRORES] Vuelva a calcular el tamaño del bloque al llamar `block.Delete`. tsdb#637
*   \[CORRECCIÓN DE ERRORES] Corrija las instantáneas inseguras con el bloqueo principal. tsdb#641
*   \[CORRECCIÓN DE ERRORES] `prometheus_tsdb_compactions_failed_total` ahora se incrementa en cualquier fallo de compactación. tsdb#613

## 2.10.0 / 2019-05-25

*   \[CAMBIO/CORRECCIÓN DE ERRORES] API: Codifique los valores de alerta como cadena para representar correctamente Inf/NaN. #5582
*   \[CARACTERÍSTICA] Expansión de plantilla: Hacer que las etiquetas externas estén disponibles como `$externalLabels` en la expansión de la plantilla de alerta y consola. #5463
*   \[CARACTERÍSTICA] TSDB: Agregar `prometheus_tsdb_wal_segment_current` métrica para el índice de segmento WAL en el que TSDB está escribiendo actualmente. tsdb#601
*   \[CARACTERÍSTICA] Raspar: Añadir `scrape_series_added` métrica por raspado. #5546
*   \[MEJORA] Discovery/kubernetes: Agregar etiquetas `__meta_kubernetes_endpoint_node_name` y `__meta_kubernetes_endpoint_hostname`. #5571
*   \[MEJORA] Discovery/azure: Agregar etiqueta `__meta_azure_machine_public_ip`. #5475
*   \[MEJORA] TSDB: Simplifique mergedPostings.Seek, lo que resulta en un mejor rendimiento si hay muchas listas de publicación. tsdb#595
*   \[MEJORA] Log tipo de sistema de archivos en el inicio. #5558
*   \[MEJORA] Cmd/promtool: Utilice solicitudes POST para Query y QueryRange. client_golang#557
*   \[MEJORA] Web: ordena las alertas por nombre de grupo. #5448
*   \[MEJORA] Plantillas de consola: Agregar variables de conveniencia `$rawParams`, `$params`, `$path`. #5463
*   \[CORRECCIÓN DE ERRORES] TSDB: No entre en pánico cuando se quede sin espacio en disco y recupérese bien de la condición. tsdb#582
*   \[CORRECCIÓN DE ERRORES] TSDB: Maneja correctamente las etiquetas vacías. tsdb#594
*   \[CORRECCIÓN DE ERRORES] TSDB: No se estrelle en una referencia de lápida desconocida. tsdb#604
*   \[CORRECCIÓN DE ERRORES] Almacenamiento/remoto: elimine las métricas específicas del gestor de colas si la cola ya no existe. #5445 #5485 #5555
*   \[CORRECCIÓN DE ERRORES] PromQL: Visualización correcta `{__name__="a"}`. #5552
*   \[CORRECCIÓN DE ERRORES] Discovery/kubernetes: Usar `service` en lugar de `ingress` como nombre de la cola de trabajo del servicio. #5520
*   \[CORRECCIÓN DE ERRORES] Discovery/azure: no entre en pánico en una máquina virtual con una IP pública. #5587
*   \[CORRECCIÓN DE ERRORES] Descubrimiento/tritón: Lea siempre el cuerpo HTTP hasta su finalización. #5596
*   \[CORRECCIÓN DE ERRORES] Web: Tipo de contenido fijo para js y css en lugar de usar `/etc/mime.types`. #5551

## 2.9.2 / 2019-04-24

*   \[CORRECCIÓN DE ERRORES] Asegúrese de que el rango de subconsulta se tenga en cuenta para la selección #5467
*   \[CORRECCIÓN DE ERRORES] Agote cada cuerpo de solicitud antes de cerrarlo #5166
*   \[CORRECCIÓN DE ERRORES] Cmd/promtool: errores de retorno de evaluaciones de reglas #5483
*   \[CORRECCIÓN DE ERRORES] Almacenamiento remoto: el intrusor de cadenas no debe entrar en pánico en la versión #5487
*   \[CORRECCIÓN DE ERRORES] Corregir la regresión de asignación de memoria en mergedPostings.Seek tsdb#586

## 2.9.1 / 2019-04-16

*   \[CORRECCIÓN DE ERRORES] Discovery/kubernetes: corregir la desinfección de etiquetas que falta #5462
*   \[CORRECCIÓN DE ERRORES] Remote_write: Evitar que el refreste coincida con la parada de llamada #5460

## 2.9.0 / 2019-04-15

Esta versión utiliza Go 1.12, que incluye un cambio en la forma en que se libera la memoria
a Linux. Esto hará que rss se informe como más alto, sin embargo, esto es inofensivo
y la memoria está disponible para el kernel cuando la necesita.

*   \[CAMBIO/MEJORA] Actualice Consul para admitir el catálogo. ServiceMultipleTags. #5151
*   \[CARACTERÍSTICA] Agregue honor_timestamps opción de raspado. #5304
*   \[MEJORA] Discovery/kubernetes: agregue etiquetas presentes para etiquetas/anotaciones. #5443
*   \[MEJORA] OpenStack SD: Agregue metaetiquetas ProjectID y UserID. #5431
*   \[MEJORA] Agregue GODEBUG y retención a la página de tiempo de ejecución. #5324 #5322
*   \[MEJORA] Agregue compatibilidad con POSTing al punto de conexión /series. #5422
*   \[MEJORA] Admite métodos PUT para las API de ciclo de vida y administración. #5376
*   \[MEJORA] Scrape: Agregue fluctuación global para el servidor de alta disponibilidad. #5181
*   \[MEJORA] Verifique la cancelación en cada paso de una evaluación de rango. #5131
*   \[MEJORA] Internado de cadenas para etiquetas y valores en la ruta de remote_write. #5316
*   \[MEJORA] No pierda la memoria caché de raspado en un raspado fallido. #5414
*   \[MEJORA] Vuelva a cargar los archivos de certificado desde el disco automáticamente. común#173
*   \[MEJORA] Utilice el formato de marca de tiempo de milisegundos de longitud fija para los registros. común#172
*   \[MEJORA] Mejoras de rendimiento para las publicaciones. tsdb#509 tsdb#572
*   \[CORRECCIÓN DE ERRORES] Escritura remota: arreglar la lectura del punto de control. #5429
*   \[CORRECCIÓN DE ERRORES] Compruebe si el valor de la etiqueta es válido al desenredar etiquetas externas de YAML. #5316
*   \[CORRECCIÓN DE ERRORES] Promparse: ordena todas las etiquetas al analizar. #5372
*   \[CORRECCIÓN DE ERRORES] Reglas de recarga: estado de copia tanto en el nombre como en las etiquetas. #5368
*   \[CORRECCIÓN DE ERRORES] Operador de exponenciación para eliminar el nombre de la métrica como resultado de la operación. #5329
*   \[CORRECCIÓN DE ERRORES] Configuración: resuelve más rutas de archivo. #5284
*   \[CORRECCIÓN DE ERRORES] Promtool: resuelve rutas relativas en archivos de prueba de alerta. #5336
*   \[CORRECCIÓN DE ERRORES] Establezca TLSHandshakeTimeout en el transporte HTTP. común#179
*   \[CORRECCIÓN DE ERRORES] Utilice fsync para ser más resistente a los bloqueos de la máquina. tsdb#573 tsdb#578
*   \[CORRECCIÓN DE ERRORES] Mantenga las series que todavía están en WAL en los puntos de control. tsdb#577
*   \[CORRECCIÓN DE ERRORES] Corrija los valores de muestra de salida para las operaciones de comparación escalar a vector. #5454

## 2.8.1 / 2019-03-28

*   \[CORRECCIÓN DE ERRORES] Mostrar las etiquetas de trabajo en `/targets` que fue retirado accidentalmente. #5406

## 2.8.0 / 2019-03-12

Esta versión utiliza el registro de escritura anticipada (WAL) para la API de remote_write. Esto actualmente causa un ligero aumento en el uso de memoria, que se abordará en futuras versiones.

*   \[CAMBIAR] La retención de tiempo predeterminada solo se usa cuando no se especifica ninguna retención basada en el tamaño. Estos son indicadores en los que la retención de tiempo se especifica mediante el indicador `--storage.tsdb.retention` y retención de tamaño por `--storage.tsdb.retention.size`. #5216
*   \[CAMBIAR] `prometheus_tsdb_storage_blocks_bytes_total` es ahora `prometheus_tsdb_storage_blocks_bytes`. prometheus/tsdb#506
*   \[CARACTERÍSTICA] \[EXPERIMENTAL] Ahora se permiten bloques de superposición de tiempo; compactación vertical y fusión de consulta vertical. Es una característica opcional que es controlada por el `--storage.tsdb.allow-overlapping-blocks` , deshabilitado de forma predeterminada. prometheus/tsdb#370
*   \[MEJORA] Utilice la API WAL for remote_write. #4588
*   \[MEJORA] Mejoras en el rendimiento de las consultas. prometheus/tsdb#531
*   \[MEJORA] Mejoras en la interfaz de usuario con la actualización a Bootstrap 4. #5226
*   \[MEJORA] Reduzca el tiempo que los Alertmanagers están en flujo cuando se vuelven a cargar. #5126
*   \[MEJORA] Limite el número de métricas que se muestran en la interfaz de usuario a 10000. #5139
*   \[MEJORA] (1) Recuerde la opción All/Unhealthy en la descripción general del objetivo al volver a cargar la página. (2) Cambie el tamaño del área de entrada de texto en la página Gráfico al hacer clic con el mouse. #5201
*   \[MEJORA] En `histogram_quantile` combinar buckets con valores le equivalentes. #5158.
*   \[MEJORA] Mostrar lista de etiquetas ofensivas en el mensaje de error en escenarios de muchos a muchos. #5189
*   \[MEJORA] Mostrar `Storage Retention` criterios vigentes sobre `/status` página. #5322
*   \[CORRECCIÓN DE ERRORES] Corregir la clasificación de grupos de reglas. #5260
*   \[CORRECCIÓN DE ERRORES] Compatibilidad con password_file y bearer_token_file en Kubernetes SD. #5211
*   \[CORRECCIÓN DE ERRORES] Scrape: detecta errores al crear clientes HTTP #5182. Agrega nuevas métricas:
    *   `prometheus_target_scrape_pools_total`
    *   `prometheus_target_scrape_pools_failed_total`
    *   `prometheus_target_scrape_pool_reloads_total`
    *   `prometheus_target_scrape_pool_reloads_failed_total`
*   \[CORRECCIÓN DE ERRORES] Solucione el pánico cuando el parámetro agregador no es literal. #5290

## 2.7.2 / 2019-03-02

*   \[CORRECCIÓN DE ERRORES] `prometheus_rule_group_last_evaluation_timestamp_seconds` ahora es una marca de tiempo unix. #5186

## 2.7.1 / 2019-01-31

Esta versión tiene una corrección para una vulnerabilidad XSS de DOM almacenado que se puede desencadenar al usar la funcionalidad del historial de consultas. Gracias a Dor Tumarkin de Checkmarx por informarlo.

*   \[CORRECCIÓN DE ERRORES/SEGURIDAD] Corregir una vulnerabilidad de DOM XSS almacenada con el historial de consultas. #5163
*   \[CORRECCIÓN DE ERRORES] `prometheus_rule_group_last_duration_seconds` ahora informa segundos en lugar de nanosegundos. #5153
*   \[CORRECCIÓN DE ERRORES] Asegúrese de que los objetivos estén ordenados de forma coherente en la página de destinos. #5161

## 2.7.0 / 2019-01-28

Estamos revirtiendo los cambios de Dockerfile introducidos en 2.6.0. Si realizó cambios en la implementación de Docker en 2.6.0, deberá revertirlos. Esta versión también agrega compatibilidad experimental para la retención basada en el tamaño del disco. Para acomodar eso estamos despreciando la bandera `storage.tsdb.retention` a favor de `storage.tsdb.retention.time`. Imprimimos una advertencia si la bandera está en uso, pero funcionará sin romperse hasta Prometheus 3.0.

*   \[CAMBIAR] Revertir Dockerfile a la versión 2.5.0. Rollback del cambio de ruptura introducido en 2.6.0. #5122
*   \[CARACTERÍSTICA] Agregue subconsultas a PromQL. #4831
*   \[CARACTERÍSTICA] \[EXPERIMENTAL] Agregue compatibilidad con la retención basada en el tamaño del disco. Tenga en cuenta que no consideramos el tamaño de WAL que podría ser significativo y también se aplica la política de retención basada en el tiempo. #5109 prometheus/tsdb#343
*   \[CARACTERÍSTICA] Agregue la bandera de origen CORS. #5011
*   \[MEJORA] Consul SD: Agregue una dirección etiquetada a los metadatos de descubrimiento. #5001
*   \[MEJORA] Kubernetes SD: agregue la IP externa del servicio y el nombre externo a los metadatos de detección. #4940
*   \[MEJORA] Azure SD: agregue compatibilidad con la autenticación de identidad administrada. #4590
*   \[MEJORA] Azure SD: agregue identificadores de inquilino y suscripción a los metadatos de detección. #4969
*   \[MEJORA] OpenStack SD: Agregue soporte para la autenticación basada en credenciales de aplicación. #4968
*   \[MEJORA] Agregue la métrica para el número de grupos de reglas cargados. #5090
*   \[CORRECCIÓN DE ERRORES] Evite las pruebas duplicadas para las pruebas unitarias de alerta. #4964
*   \[CORRECCIÓN DE ERRORES] No dependa del orden dado al comparar muestras en pruebas unitarias de alerta. #5049
*   \[CORRECCIÓN DE ERRORES] Asegúrate de que el período de retención no se desborde. #5112
*   \[CORRECCIÓN DE ERRORES] Asegúrese de que los bloques no sean muy grandes. #5112
*   \[CORRECCIÓN DE ERRORES] No genere bloques sin muestras. prometheus/tsdb#374
*   \[CORRECCIÓN DE ERRORES] Reintroducir la métrica para las corrupciones wal. prometheus/tsdb#473

## 2.6.1 / 2019-01-15

*   \[CORRECCIÓN DE ERRORES] Azure SD: Corrección de la detección que se atasca a veces. #5088
*   \[CORRECCIÓN DE ERRORES] Marathon SD: Uso `Tasks.Ports` cuando `RequirePorts` es `false`. #5026
*   \[CORRECCIÓN DE ERRORES] Promtool: Corrija los errores de "muestra fuera de orden" al probar las reglas. #5069

## 2.6.0 / 2018-12-17

*   \[CAMBIAR] Elimine los indicadores predeterminados del punto de entrada del contenedor, ejecute Prometheus desde `/etc/prometheus` y vincule el directorio de almacenamiento a `/etc/prometheus/data`. #4976
*   \[CAMBIAR] Promtool: Retire el `update` mandar. #3839
*   \[CARACTERÍSTICA] Agregue el formato de registro JSON a través del `--log.format` bandera. #4876
*   \[CARACTERÍSTICA] API: Agregue el extremo /api/v1/labels para obtener todos los nombres de etiqueta. #4835
*   \[CARACTERÍSTICA] Web: Permitir la configuración del título de la página a través del `--web.ui-title` bandera. #4841
*   \[MEJORA] Agregar `prometheus_tsdb_lowest_timestamp_seconds`, `prometheus_tsdb_head_min_time_seconds` y `prometheus_tsdb_head_max_time_seconds` Métricas. #4888
*   \[MEJORA] Agregar `rule_group_last_evaluation_timestamp_seconds` métrico. #4852
*   \[MEJORA] Agregar `prometheus_template_text_expansion_failures_total` y `prometheus_template_text_expansions_total` Métricas. #4747
*   \[MEJORA] Establezca un encabezado User-Agent coherente en las solicitudes salientes. #4891
*   \[MEJORA] Azure SD: error en el momento de la carga cuando faltan parámetros de autenticación. #4907
*   \[MEJORA] EC2 SD: agregue el nombre DNS privado de la máquina a los metadatos de detección. #4693
*   \[MEJORA] EC2 SD: Agregue la plataforma del sistema operativo a los metadatos de descubrimiento. #4663
*   \[MEJORA] Kubernetes SD: agregue la fase del pod a los metadatos de descubrimiento. #4824
*   \[MEJORA] Kubernetes SD: Registra mensajes de Kubernetes. #4931
*   \[MEJORA] Promtool: Recopila perfiles de CPU y seguimiento. #4897
*   \[MEJORA] Promtool: Admite la salida de escritura como JSON. #4848
*   \[MEJORA] Lectura remota: devuelva los datos disponibles si la lectura remota falla parcialmente. #4832
*   \[MEJORA] Escritura remota: mejore el rendimiento de la cola. #4772
*   \[MEJORA] Escritura remota: agregue min_shards parámetro para establecer el número mínimo de particiones. #4924
*   \[MEJORA] TSDB: Mejorar la lectura de WAL. #4953
*   \[MEJORA] TSDB: Mejoras de memoria. #4953
*   \[MEJORA] Web: Trazas de pila de registros en pánico. #4221
*   \[MEJORA] Interfaz de usuario web: agregue el botón Copiar al portapapeles para la configuración. #4410
*   \[MEJORA] Interfaz de usuario web: admite consultas de consola en momentos específicos. #4764
*   \[MEJORA] Interfaz de usuario web: agrupar destinos por trabajo y luego instancia. #4898 #4806
*   \[CORRECCIÓN DE ERRORES] Deduplicar etiquetas de controlador para métricas HTTP. #4732
*   \[CORRECCIÓN DE ERRORES] Corrija los consultadores filtrados que causan que los apagados se bloqueen. #4922
*   \[CORRECCIÓN DE ERRORES] Corrija los pánicos de carga de configuración en elementos de división de puntero nulo. #4942
*   \[CORRECCIÓN DE ERRORES] API: omita correctamente los destinos que no coinciden en /api/v1/targets/metadata. #4905
*   \[CORRECCIÓN DE ERRORES] API: Mejor redondeo para las marcas de tiempo de consultas entrantes. #4941
*   \[CORRECCIÓN DE ERRORES] Azure SD: Corregir el pánico. #4867
*   \[CORRECCIÓN DE ERRORES] Plantillas de consola: corrija el cursor cuando la métrica tenga un valor nulo. #4906
*   \[CORRECCIÓN DE ERRORES] Detección: elimine todos los destinos cuando la configuración de raspado se vacíe. #4819
*   \[CORRECCIÓN DE ERRORES] Marathon SD: Arregla las conexiones filtradas. #4915
*   \[CORRECCIÓN DE ERRORES] Marathon SD: Utilice el miembro 'hostPort' de portMapping para construir puntos finales de destino. #4887
*   \[CORRECCIÓN DE ERRORES] PromQL: Arregle una fuga de goroutine en el lexer/analizador. #4858
*   \[CORRECCIÓN DE ERRORES] Scrape: Pase a través del tipo de contenido para la salida no comprimida. #4912
*   \[CORRECCIÓN DE ERRORES] Scrape: Solucione el punto muerto en el administrador del scrape. #4894
*   \[CORRECCIÓN DE ERRORES] Raspado: Raspa objetivos a intervalos fijos incluso después de que Prometheus se reinicie. #4926
*   \[CORRECCIÓN DE ERRORES] TSDB: Admite instantáneas restauradas, incluido el cabezal correctamente. #4953
*   \[CORRECCIÓN DE ERRORES] TSDB: Repare WAL cuando se rompa el último registro de un segmento. #4953
*   \[CORRECCIÓN DE ERRORES] TSDB: Arregle lectores de archivos sin cerrar en sistemas Windows. #4997
*   \[CORRECCIÓN DE ERRORES] Web: Evite que el proxy se conecte al servidor gRPC local. #4572

## 2.5.0 / 2018-11-06

*   \[CAMBIAR] Agrupe los destinos mediante la configuración de raspado en lugar del nombre del trabajo. #4806 #4526
*   \[CAMBIAR] Maratón SD: Varios cambios para adaptarse al Maratón 1.5+. #4499
*   \[CAMBIAR] Descubrimiento: Split `prometheus_sd_discovered_targets` métrica por raspado y notificación (Alertmanager SD), así como por sección en la configuración respectiva. #4753
*   \[CARACTERÍSTICA] Agregue compatibilidad con OpenMetrics para el raspado (EXPERIMENTAL). #4700
*   \[CARACTERÍSTICA] Agregue pruebas unitarias para las reglas. #4350
*   \[CARACTERÍSTICA] Hacer que el número máximo de muestras por consulta sea configurable a través de `--query.max-samples` bandera. #4513
*   \[CARACTERÍSTICA] Hacer que el número máximo de lecturas remotas simultáneas sea configurable a través de `--storage.remote.read-concurrent-limit` bandera. #4656
*   \[MEJORA] Soporta plataforma s390x para Linux. #4605
*   \[MEJORA] API: Agregar `prometheus_api_remote_read_queries` seguimiento de métricas actualmente ejecutado o en espera de solicitudes de API de lectura remota. #4699
*   \[MEJORA] Lectura remota: Agregar `prometheus_remote_storage_remote_read_queries` seguimiento métrico actualmente en consultas de lectura remota en vuelo. #4677
*   \[MEJORA] Lectura remota: uso reducido de memoria. #4655
*   \[MEJORA] Descubrimiento: Agregar `prometheus_sd_discovered_targets`, `prometheus_sd_received_updates_total`, `prometheus_sd_updates_delayed_total`y `prometheus_sd_updates_total` métricas para el subsistema de descubrimiento. #4667
*   \[MEJORA] Descubrimiento: Mejore el rendimiento de las actualizaciones previamente lentas de los cambios de los objetivos. #4526
*   \[MEJORA] Kubernetes SD: Agregue métricas extendidas. #4458
*   \[MEJORA] OpenStack SD: Admite la detección de instancias de todos los proyectos. #4682
*   \[MEJORA] OpenStack SD: Descubre todas las interfaces. #4649
*   \[MEJORA] OpenStack SD: Soporte `tls_config` para el cliente HTTP utilizado. #4654
*   \[MEJORA] Triton SD: Agregue la capacidad de filtrar triton_sd objetivos por grupos predefinidos. #4701
*   \[MEJORA] Interfaz de usuario web: evite la revisión ortográfica del explorador en el campo de expresión. #4728
*   \[MEJORA] Interfaz de usuario web: agregue la duración del raspado y el último tiempo de evaluación en las páginas de destinos y reglas. #4722
*   \[MEJORA] Interfaz de usuario web: mejore la vista de reglas ajustando líneas. #4702
*   \[MEJORA] Reglas: Error en el momento de la carga para plantillas no válidas, en lugar de en el momento de la evaluación. #4537
*   \[MEJORA] TSDB: Agregue métricas para las operaciones wal. #4692
*   \[CORRECCIÓN DE ERRORES] Cambie el over_time máx/min para manejar los NaN correctamente. #4386
*   \[CORRECCIÓN DE ERRORES] Compruebe el nombre de la etiqueta para `count_values` Función PromQL. #4585
*   \[CORRECCIÓN DE ERRORES] Asegúrese de que los vectores y matrices no contengan conjuntos de etiquetas idénticos. #4589

## 2.4.3 / 2018-10-04

*   \[CORRECCIÓN DE ERRORES] Solucione el pánico al usar la API de EC2 personalizada para SD #4672
*   \[CORRECCIÓN DE ERRORES] Solucione el pánico cuando Zookeeper SD no puede conectarse a los servidores #4669
*   \[CORRECCIÓN DE ERRORES] Convertir el skip_head un parámetro opcional para la API de instantáneas #4674

## 2.4.2 / 2018-09-21

La última versión no tenía corrección de errores incluida debido a un error de proveedor.

*   \[CORRECCIÓN DE ERRORES] Manejar correctamente las corrupciones wal prometheus/tsdb#389
*   \[CORRECCIÓN DE ERRORES] Manejar correctamente las migraciones WAL en Windows prometheus/tsdb#392

## 2.4.1 / 2018-09-19

*   \[MEJORA] Nuevas métricas de TSDB prometheus/tsdb#375 prometheus/tsdb#363
*   \[CORRECCIÓN DE ERRORES] Procesar la interfaz de usuario correctamente para Windows #4616

## 2.4.0 / 2018-09-11

Esta versión incluye varias correcciones de errores y características. Además, la implementación de WAL se ha reescrito, por lo que el almacenamiento no es compatible con el futuro. El almacenamiento de Prometheus 2.3 funcionará en 2.4, pero no al revés.

*   \[CAMBIAR] Reducir los reintentos predeterminados de escritura remota #4279
*   \[CAMBIAR] Quitar el extremo /heap #4460
*   \[CARACTERÍSTICA] Persistir el estado de alerta 'para' en los reinicios #4061
*   \[CARACTERÍSTICA] Agregar API que proporciona metadatos por métrica de destino #4183
*   \[CARACTERÍSTICA] Agregar API que proporciona reglas de grabación y alerta #4318 #4501
*   \[MEJORA] Nueva implementación de WAL para TSDB. Reenvíos incompatibles con WAL anteriores.
*   \[MEJORA] Mostrar errores de evaluación de reglas en la interfaz de usuario #4457
*   \[MEJORA] Reenvío del acelerador de alertas a Alertmanager #4538
*   \[MEJORA] Enviar EndsAt junto con la alerta a Alertmanager #4550
*   \[MEJORA] Limitar las muestras devueltas por el extremo de lectura remota #4532
*   \[MEJORA] Limite la lectura de datos a través de la lectura remota #4239
*   \[MEJORA] Coalesce configuraciones SD idénticas #3912
*   \[MEJORA] `promtool`: Agregar nuevos comandos para depurar y consultar #4247 #4308 #4346 #4454
*   \[MEJORA] Ejemplos de consola de actualización para node_exporter v0.16.0 #4208
*   \[MEJORA] Optimizar las agregaciones de PromQL #4248
*   \[MEJORA] Lectura remota: Agregar desplazamiento a las sugerencias #4226
*   \[MEJORA] `consul_sd`: Agregar compatibilidad con el campo ServiceMeta #4280
*   \[MEJORA] `ec2_sd`: Mantener el orden de subnet_id etiqueta #4405
*   \[MEJORA] `ec2_sd`: Agregue compatibilidad con puntos de conexión personalizados para admitir API compatibles con EC2 #4333
*   \[MEJORA] `ec2_sd`: Añadir instance_owner etiqueta #4514
*   \[MEJORA] `azure_sd`: Agregar compatibilidad con el descubrimiento de VMSS y múltiples entornos #4202 #4569
*   \[MEJORA] `gce_sd`: Añadir instance_id etiqueta #4488
*   \[MEJORA] Prohibir que los robots respetuosos de las reglas indexen #4266
*   \[MEJORA] Registrar los límites de memoria virtual en el inicio #4418
*   \[CORRECCIÓN DE ERRORES] Espere a que se detenga la detección de servicios antes de salir de #4508
*   \[CORRECCIÓN DE ERRORES] Renderizar las configuraciones SD correctamente #4338
*   \[CORRECCIÓN DE ERRORES] Solo agregue LookbackDelta a los selectores vectoriales #4399
*   \[CORRECCIÓN DE ERRORES] `ec2_sd`: Manejar el puntero nulo de pánico #4469
*   \[CORRECCIÓN DE ERRORES] `consul_sd`: Deje de filtrar conexiones #4443
*   \[CORRECCIÓN DE ERRORES] Use etiquetas con plantillas también para identificar alertas #4500
*   \[CORRECCIÓN DE ERRORES] Reducir los errores de coma flotante en stddev y funciones relacionadas #4533
*   \[CORRECCIÓN DE ERRORES] Errores de registro al codificar respuestas #4359

## 2.3.2 / 2018-07-12

*   \[CORRECCIÓN DE ERRORES] Corregir varios errores de tsdb #4369
*   \[CORRECCIÓN DE ERRORES] Reordene el inicio y el apagado para evitar pánicos. #4321
*   \[CORRECCIÓN DE ERRORES] Salir con código distinto de cero en el error #4296
*   \[BUGFIX] discovery/kubernetes/ingress: fix scheme discovery #4329
*   \[CORRECCIÓN DE ERRORES] Arreglar raza en zookeeper sd #4355
*   \[CORRECCIÓN DE ERRORES] Mejor manejo del tiempo de espera en promql #4291 #4300
*   \[CORRECCIÓN DE ERRORES] Propagar errores al seleccionar series del tsdb #4136

## 2.3.1 / 2018-06-19

*   \[CORRECCIÓN DE ERRORES] Evite el bucle infinito en valores de NaN duplicados. #4275
*   \[CORRECCIÓN DE ERRORES] Corregir la deferencia del puntero nulo cuando se usan varios puntos de enlace de API #4282
*   Configuración de \[BUGFIX]: establecer el índice de origen del grupo de destino durante la desmarañación #4245
*   \[BUGFIX] descubrimiento / archivo: corregir registro # 4178
*   \[BUGFIX] kubernetes_sd: corregir el filtrado de espacio de nombres #4285
*   \[BUGFIX] web: restaurar el comportamiento del prefijo de ruta antiguo #4273
*   \[BUGFIX] web: eliminar encabezados de seguridad agregados en 2.3.0 #4259

## 2.3.0 / 2018-06-05

*   \[CAMBIAR] `marathon_sd`: uso `auth_token` y `auth_token_file` para la autenticación basada en tokens en lugar de `bearer_token` y `bearer_token_file` respectivamente.
*   \[CAMBIAR] Se han cambiado los nombres de las métricas del servidor HTTP
*   \[CARACTERÍSTICA] Agregar comandos de consulta a promtool
*   \[CARACTERÍSTICA] Agregar encabezados de seguridad a las respuestas del servidor HTTP
*   \[CARACTERÍSTICA] Pasar sugerencias de consulta a través de la API de lectura remota
*   \[CARACTERÍSTICA] Las contraseñas de autenticación básicas ahora se pueden configurar a través de un archivo en toda la configuración
*   \[MEJORA] Optimice la serialización de PromQL y API para el uso y las asignaciones de memoria
*   \[MEJORA] Limitar el número de destinos eliminados en la interfaz de usuario web
*   \[MEJORA] La detección de servicios de Consul y EC2 permite el uso de filtrado del lado del servidor para mejorar el rendimiento
*   \[MEJORA] Agregar configuración de filtrado avanzada a la detección de servicios ec2
*   \[MEJORA] `marathon_sd`: agrega compatibilidad con la autenticación básica y de portador, además de todas las demás opciones comunes del cliente HTTP (configuración TLS, URL de proxy, etc.)
*   \[MEJORA] Proporcionar metadatos y etiquetas de tipo de máquina en la detección de servicios GCE
*   \[MEJORA] Agregar el tipo y el nombre del controlador de pod a los datos de detección del servicio Kubernetes
*   \[MEJORA] Mover TSDB a un archivo de registro basado en bandadas que funcione con contenedores de Docker
*   \[CORRECCIÓN DE ERRORES] Propagar correctamente los errores de almacenamiento en PromQL
*   \[CORRECCIÓN DE ERRORES] Corregir prefijo de ruta para páginas web
*   \[CORRECCIÓN DE ERRORES] Reparar fuga de goroutine en el descubrimiento del servicio Consul
*   \[CORRECCIÓN DE ERRORES] Arreglar carreras en el gestor de raspaduras
*   \[CORRECCIÓN DE ERRORES] Corregir OOM para k muy grandes en consultas TopK() de PromQL
*   \[CORRECCIÓN DE ERRORES] Hacer que la escritura remota sea más resistente a los receptores no disponibles
*   \[CORRECCIÓN DE ERRORES] Realizar el apagado de escritura remota de forma limpia
*   \[CORRECCIÓN DE ERRORES] No filtre archivos sobre errores en la limpieza de lápidas de TSDB
*   \[CORRECCIÓN DE ERRORES] Las expresiones menos unarias ahora quitan el nombre de la métrica de los resultados
*   \[CORRECCIÓN DE ERRORES] Corregir un error que provocaba una cantidad incorrecta de muestras consideradas para expresiones de intervalo de tiempo

## 2.2.1 / 2018-03-13

*   \[CORRECCIÓN DE ERRORES] Corregir la pérdida de datos en TSDB en la compactación
*   \[CORRECCIÓN DE ERRORES] Detener correctamente el temporizador en la ruta de escritura remota
*   \[CORRECCIÓN DE ERRORES] Corregir el interbloqueo desencadenado por la carga de la página de destinos
*   \[CORRECCIÓN DE ERRORES] Corregir el almacenamiento en búfer incorrecto de muestras en consultas de selección de rango
*   \[CORRECCIÓN DE ERRORES] Manejar archivos de índice grandes en Windows correctamente

## 2.2.0 / 2018-03-08

*   \[CAMBIAR] Cambie el nombre del archivo SD mtime metric.
*   \[CAMBIAR] Envíe la actualización de destino en la IP del pod vacía en kubernetes SD.
*   \[CARACTERÍSTICA] Agregue el extremo de la API para los indicadores.
*   \[CARACTERÍSTICA] Agregue el punto de enlace de la API para los destinos eliminados.
*   \[CARACTERÍSTICA] Mostrar anotaciones en la página de alertas.
*   \[CARACTERÍSTICA] Agregue la opción de omitir los datos del cabezal al tomar instantáneas.
*   \[MEJORA] Mejora del rendimiento de la federación.
*   \[MEJORA] Lea el archivo de token del portador en cada raspado.
*   \[MEJORA] Mejorar el tipo de cabeza en `/graph` página.
*   \[MEJORA] Cambiar el formato de archivo de regla.
*   \[MEJORA] Establecer el servidor consul predeterminado en `localhost:8500`.
*   \[MEJORA] Agregue Alertmanagers eliminados al punto de enlace de información de API.
*   \[MEJORA] Agregue la metaetiqueta de tipo de sistema operativo a Azure SD.
*   \[MEJORA] Valide los campos obligatorios en la configuración SD.
*   \[CORRECCIÓN DE ERRORES] Evite el desbordamiento de la pila en la recursión profunda en TSDB.
*   \[CORRECCIÓN DE ERRORES] Lea correctamente los desplazamientos en archivos de índice que sean superiores a 4 GB.
*   \[CORRECCIÓN DE ERRORES] Corregir el comportamiento de raspado de etiquetas vacías.
*   \[CORRECCIÓN DE ERRORES] Suelte el nombre de la métrica para el modificador bool.
*   \[CORRECCIÓN DE ERRORES] Arreglar carreras en el descubrimiento.
*   \[CORRECCIÓN DE ERRORES] Corrija los puntos finales de Kubernetes SD para subconjuntos vacíos.
*   \[CORRECCIÓN DE ERRORES] Limite las actualizaciones de los proveedores de SD, lo que provocó un aumento en el uso y las asignaciones de la CPU.
*   \[CORRECCIÓN DE ERRORES] Solucione el problema de recarga de bloques de TSDB.
*   \[CORRECCIÓN DE ERRORES] Arreglar la impresión PromQL de vacío `without()`.
*   \[CORRECCIÓN DE ERRORES] No restablezcas FiredAt para alertas inactivas.
*   \[CORRECCIÓN DE ERRORES] Corrija los cambios erróneos en la versión del archivo y repare los datos existentes.

## 2.1.0 / 2018-01-19

*   \[CARACTERÍSTICA] Nueva interfaz de usuario de detección de servicios que muestra las etiquetas antes y después de volver a etiquetar.
*   \[CARACTERÍSTICA] Nuevas API de administración agregadas a v1 para eliminar, instantáneas y eliminar lápidas.
*   \[MEJORA] El autocompletado de la interfaz de usuario del gráfico ahora incluye sus consultas anteriores.
*   \[MEJORA] La federación es ahora mucho más rápida para un gran número de series.
*   \[MEJORA] Se agregaron nuevas métricas para medir los tiempos de las reglas.
*   \[MEJORA] Tiempos de evaluación de reglas agregados a la interfaz de usuario de reglas.
*   \[MEJORA] Se agregaron métricas para medir el tiempo de modificación de los archivos SD de archivos.
*   \[MEJORA] Kubernetes SD ahora incluye POD UID en los metadatos de descubrimiento.
*   \[MEJORA] Las API de consulta ahora devuelven estadísticas opcionales sobre los tiempos de ejecución de la consulta.
*   \[MEJORA] El índice ahora ya no tiene el límite de tamaño de 4GiB y también es más pequeño.
*   \[CORRECCIÓN DE ERRORES] Lectura remota `read_recent` ahora es false de forma predeterminada.
*   \[CORRECCIÓN DE ERRORES] Pase la configuración correcta a cada Alertmanager (AM) cuando utilice varias configuraciones de AM.
*   \[CORRECCIÓN DE ERRORES] Corrija los no emparejadores que no seleccionan series con etiquetas sin establecer.
*   \[CORRECCIÓN DE ERRORES] tsdb: Solucione el pánico ocasional en el bloqueo de la cabeza.
*   \[CORRECCIÓN DE ERRORES] tsdb: Cierre los archivos antes de eliminarlos para solucionar problemas de retención en Windows y NFS.
*   \[CORRECCIÓN DE ERRORES] tsdb: Limpie y no vuelva a intentar compactaciones defectuosas.
*   \[CORRECCIÓN DE ERRORES] tsdb: Cierre WAL mientras se apaga.

## 2.0.0 / 2017-11-08

Esta versión incluye un almacenamiento completamente reescrito, un gran rendimiento
mejoras, pero también muchos cambios incompatibles con versiones anteriores. Para más información
información, lea la publicación del blog del anuncio y la guía de migración.

<https://prometheus.io/blog/2017/11/08/announcing-prometheus-2-0/>
<https://prometheus.io/docs/prometheus/2.0/migration/>

*   \[CAMBIAR] Capa de almacenamiento completamente reescrita, con WAL. Esto no es compatible con versiones anteriores del almacenamiento 1.x, y muchos indicadores han cambiado/desaparecido.
*   \[CAMBIAR] Nuevo comportamiento rancio. Las series ahora marcadas como obsoletas después de que los raspados de objetivos ya no los devuelven, y poco después los objetivos desaparecen del descubrimiento de servicios.
*   \[CAMBIAR] Los archivos de reglas usan la sintaxis YAML ahora. Herramienta de conversión agregada a promtool.
*   \[CAMBIAR] Quitado `count_scalar`, `drop_common_labels` funciones y `keep_common` modificador de PromQL.
*   \[CAMBIAR] Analizador de formato de exposición reescrito con un rendimiento mucho mayor. El formato de exposición Protobuf ya no es compatible.
*   \[CAMBIAR] Plantillas de consola de ejemplo actualizadas para nuevos nombres de almacenamiento y métricas. Ejemplos distintos del exportador de nodos y Prometheus eliminados.
*   \[CAMBIAR] Las API de administración y ciclo de vida ahora deshabilitadas de forma predeterminada, se pueden volver a habilitar a través de indicadores
*   \[CAMBIAR] Las banderas cambiaron a usar Kingpin, todas las banderas ahora son --flagname en lugar de -flagname.
*   \[CARACTERÍSTICA/CAMBIO] La lectura remota se puede configurar para que no lea los datos que están disponibles localmente. Esto está habilitado de forma predeterminada.
*   \[CARACTERÍSTICA] Las reglas se pueden agrupar ahora. Las reglas dentro de un grupo de reglas se ejecutan secuencialmente.
*   \[CARACTERÍSTICA] Se agregaron apis GRPC experimentales
*   \[CARACTERÍSTICA] Agregue la función timestamp() a PromQL.
*   \[MEJORA] Elimine la lectura remota de la ruta de consulta si no hay ningún almacenamiento remoto configurado.
*   \[MEJORA] Tiempo de espera del cliente HTTP de Bump Consul para que no coincida con el tiempo de espera de la supervisión de Consul SD.
*   \[MEJORA] Go-conntrack agregado para proporcionar métricas de conexión HTTP.
*   \[CORRECCIÓN DE ERRORES] Arreglar fuga de conexión en Consul SD.

## 1.8.2 / 2017-11-04

*   \[CORRECCIÓN DE ERRORES] Detección de servicios EC2: no se bloquee si las etiquetas están vacías.

## 1.8.1 / 2017-10-19

*   \[CORRECCIÓN DE ERRORES] Manejar correctamente las etiquetas externas en el extremo de lectura remota

## 1.8.0 / 2017-10-06

*   \[CAMBIAR] Enlace de vínculos de regla a la *Consola* en lugar de la ficha *Gráfico* pestaña a
    no desencadenar consultas de rango costosas de forma predeterminada.
*   \[CARACTERÍSTICA] Capacidad para actuar como un punto final de lectura remota para otros Prometheus
    Servidores.
*   \[CARACTERÍSTICA] K8s SD: Soporte para el descubrimiento de entradas.
*   \[CARACTERÍSTICA] Consul SD: Soporte para metadatos de nodo.
*   \[CARACTERÍSTICA] Openstack SD: Soporte para el descubrimiento de hipervisores.
*   \[CARACTERÍSTICA] Exponer la configuración actual de Prometheus a través de `/status/config`.
*   \[CARACTERÍSTICA] Permitir la contracción de trabajos en `/targets` página.
*   \[CARACTERÍSTICA] Agregar `/-/healthy` y `/-/ready` Extremos.
*   \[CARACTERÍSTICA] Agregue compatibilidad con combinaciones de colores a las plantillas de consola.
*   \[MEJORA] Las conexiones de almacenamiento remoto utilizan HTTP keep-alive.
*   \[MEJORA] Se ha mejorado el registro sobre el almacenamiento remoto.
*   \[MEJORA] Validación de URL relajada.
*   \[MEJORA] Openstack SD: Maneja instancias sin IP.
*   \[MEJORA] Haga que el administrador de colas de almacenamiento remoto sea configurable.
*   \[MEJORA] Valide las métricas devueltas desde la lectura remota.
*   \[MEJORA] EC2 SD: Establezca una región predeterminada.
*   \[MEJORA] Se ha cambiado el vínculo de ayuda a `https://prometheus.io/docs`.
*   \[CORRECCIÓN DE ERRORES] Solucionar el problema de precisión de coma flotante en `deriv` función.
*   \[CORRECCIÓN DE ERRORES] Corregir puntos finales pprof cuando -web.route-prefix o -web.external-url es
    usado.
*   \[CORRECCIÓN DE ERRORES] Arreglar el manejo de `null` grupos objetivo en SD basada en archivos.
*   \[CORRECCIÓN DE ERRORES] Establezca la marca de tiempo de ejemplo en las funciones PromQL relacionadas con la fecha.
*   \[CORRECCIÓN DE ERRORES] Aplique el prefijo de ruta para redirigir desde la URL del gráfico obsoleto.
*   \[CORRECCIÓN DE ERRORES] Se han corregido las pruebas en MS Windows.
*   \[CORRECCIÓN DE ERRORES] Compruebe si hay UTF-8 no válido en los valores de etiqueta después de volver a etiquetar.

## 1.7.2 / 2017-09-26

*   \[CORRECCIÓN DE ERRORES] Quitar correctamente todos los destinos de la detección de servicios DNS si el
    la consulta DNS correspondiente se realiza correctamente y devuelve un resultado vacío.
*   \[CORRECCIÓN DE ERRORES] Analice correctamente la entrada de resolución en el explorador de expresiones.
*   \[CORRECCIÓN DE ERRORES] Utilice UTC de forma coherente en el selector de fecha del explorador de expresiones.
*   \[CORRECCIÓN DE ERRORES] Maneje correctamente varios puertos en la detección de servicios de Marathon.
*   \[CORRECCIÓN DE ERRORES] Corrija el escape HTML para que las plantillas HTML se compilen con Go1.9.
*   \[CORRECCIÓN DE ERRORES] Evite que el número de fragmentos de escritura remota se vuelva negativo.
*   \[CORRECCIÓN DE ERRORES] En los gráficos creados por el explorador de expresiones, renderizar muy grande
    y números pequeños de una manera legible.
*   \[CORRECCIÓN DE ERRORES] Solucione un problema de iterador que rara vez ocurre en fragmentos codificados en varbit.

## 1.7.1 / 2017-06-12

*   \[CORRECCIÓN DE ERRORES] Arregle la redirección de prefijo doble.

## 1.7.0 / 2017-06-06

*   \[CAMBIAR] Comprima las solicitudes y respuestas de almacenamiento remoto con unframed/raw snappy.
*   \[CAMBIAR] Ellide correctamente los secretos en config.
*   \[CARACTERÍSTICA] Agregar detección de servicios OpenStack.
*   \[CARACTERÍSTICA] Agregue la capacidad de limitar la detección de servicios de Kubernetes a ciertos espacios de nombres.
*   \[CARACTERÍSTICA] Agregue una métrica para el número descubierto de Alertmanagers.
*   \[MEJORA] Imprimir información del sistema (uname) en el arranque.
*   \[MEJORA] Mostrar huecos en los gráficos en el explorador de expresiones.
*   \[MEJORA] Promtool linter comprueba la nomenclatura del contador y las etiquetas más reservadas.
*   \[CORRECCIÓN DE ERRORES] Arreglar el descubrimiento de Mesos roto.
*   \[CORRECCIÓN DE ERRORES] Corrija la redirección cuando se establece la URL externa.
*   \[CORRECCIÓN DE ERRORES] Corregir la mutación de los elementos de alerta activos mediante el notificador.
*   \[CORRECCIÓN DE ERRORES] Corrija el manejo de errores HTTP para la escritura remota.
*   \[CORRECCIÓN DE ERRORES] Arreglar compilaciones para Solaris/Illumos.
*   \[CORRECCIÓN DE ERRORES] Corregir la comprobación de desbordamiento en la configuración global.
*   \[CORRECCIÓN DE ERRORES] Solucione el problema de informes a nivel de registro.
*   \[CORRECCIÓN DE ERRORES] Fix ZooKeeper ServerSet Discovery puede dejar de estar sincronizado.

## 1.6.3 / 2017-05-18

*   \[CORRECCIÓN DE ERRORES] Corregir los destinos de Alertmanager que desaparecen en la detección de Alertmanager.
*   \[CORRECCIÓN DE ERRORES] Soluciona el pánico con remote_write en ARMv7.
*   \[CORRECCIÓN DE ERRORES] Corrija los gráficos apilados para adaptar los valores mínimos/máximos.

## 1.6.2 / 2017-05-11

*   \[CORRECCIÓN DE ERRORES] Corregir una posible pérdida de memoria en el descubrimiento de servicios de Kubernetes

## 1.6.1 / 2017-04-19

*   \[CORRECCIÓN DE ERRORES] No entre en pánico si el almacenamiento no tiene FP incluso después de la espera inicial

## 1.6.0 / 2017-04-14

*   \[CAMBIAR] Reemplazó las implementaciones de escritura remota para varios backends por un
    Interfaz de escritura genérica con implementación de adaptador de ejemplo para varios
    backends. Tenga en cuenta que tanto la escritura remota anterior como la actual
    las implementaciones son **experimental**.
*   \[CARACTERÍSTICA] Nueva bandera `-storage.local.target-heap-size` para contarle a Prometeo sobre
    el tamaño de montón deseado. Esto deja en desuso las banderas
    `-storage.local.memory-chunks` y `-storage.local.max-chunks-to-persist`,
    que se mantienen para la compatibilidad con versiones anteriores.
*   \[CARACTERÍSTICA] Agregar `check-metrics` Para `promtool` para pelar nombres de métricas.
*   \[CARACTERÍSTICA] Agregue el descubrimiento de Joyent Triton.
*   \[CARACTERÍSTICA] `X-Prometheus-Scrape-Timeout-Seconds` encabezado en raspado HTTP
    Solicitudes.
*   \[CARACTERÍSTICA] Interfaz de lectura remota, incluido el ejemplo de InfluxDB. **Experimental.**
*   \[CARACTERÍSTICA] Habilite Consul SD para conectarse a través de TLS.
*   \[CARACTERÍSTICA] Marathon SD admite múltiples puertos.
*   \[CARACTERÍSTICA] Marathon SD admite el token de portador para la autenticación.
*   \[CARACTERÍSTICA] Tiempo de espera personalizado para consultas.
*   \[CARACTERÍSTICA] Exponer `buildQueryUrl` en `graph.js`.
*   \[CARACTERÍSTICA] Agregar `rickshawGraph` del objeto graph de la consola
    Plantillas.
*   \[CARACTERÍSTICA] Nuevas métricas exportadas por el propio Prometheus:
    *   Resumen `prometheus_engine_query_duration_seconds`
    *   Mostrador `prometheus_evaluator_iterations_missed_total`
    *   Mostrador `prometheus_evaluator_iterations_total`
    *   Calibre `prometheus_local_storage_open_head_chunks`
    *   Calibre `prometheus_local_storage_target_heap_size`
*   \[MEJORA] Reduzca el tiempo de apagado interrumpiendo un punto de control continuo
    antes de iniciar el punto de control final.
*   \[MEJORA] Ajuste automático de los tiempos entre puntos de control para limitar el tiempo que se pasa en
    punto de control al 50%.
*   \[MEJORA] La recuperación de fallos mejorada se adapta mejor a ciertos índices
    Corrupciones.
*   \[MEJORA] La representación gráfica se ocupa mejor de las series temporales constantes.
*   \[MEJORA] Vuelva a intentar escrituras remotas en errores recuperables.
*   \[MEJORA] Desaloje los descriptores de fragmentos no utilizados durante la recuperación de bloqueos para limitar
    uso de memoria.
*   \[MEJORA] Uso más fluido del disco durante el mantenimiento en serie.
*   \[MEJORA] Destinos en la página de destinos ordenados por instancia dentro de un trabajo.
*   \[MEJORA] Ordenar etiquetas en federación.
*   \[MEJORA] Poner `GOGC=40` de forma predeterminada, lo que da como resultado una memoria mucho mejor
    utilización al precio de un uso de CPU ligeramente mayor. Si `GOGC` está configurado por
    el usuario, sigue siendo honrado como de costumbre.
*   \[MEJORA] Cierre los trozos de cabeza después de estar inactivo durante la duración de la
    delta obsoleto configurado. Esto ayuda a persistir y desalojar el trozo de cabeza de
    series obsoletas más rápidamente.
*   \[MEJORA] Comprobación más estricta de la configuración de reetiquetado.
*   \[MEJORA] Destructores de caché para contenido web estático.
*   \[MEJORA] Envíe el encabezado de agente de usuario específico de Prometheus durante los raspados.
*   \[MEJORA] Rendimiento mejorado del corte de retención de series.
*   \[MEJORA] Mitigar el impacto de la ingestión de muestras no atómicas en
    `histogram_quantile` haciendo cumplir los cubos para que sean monótonos.
*   \[MEJORA] Binarios lanzados creados con Go 1.8.1.
*   \[CORRECCIÓN DE ERRORES] Enviar `instance=""` con federación si `instance` no establecido.
*   \[CORRECCIÓN DE ERRORES] Actualización a nuevo `client_golang` para deshacerse de los cuantiles no deseados
    métricas en resúmenes.
*   \[CORRECCIÓN DE ERRORES] Introduzca varios protectores adicionales contra la corrupción de datos.
*   \[CORRECCIÓN DE ERRORES] Marcar almacenamiento sucio e incrementar
    `prometheus_local_storage_persist_errors_total` en todos los errores relevantes.
*   \[CORRECCIÓN DE ERRORES] Propague los errores de almacenamiento como 500 en la API HTTP.
*   \[CORRECCIÓN DE ERRORES] Corrija el desbordamiento de int64 en marcas de tiempo en la API HTTP.
*   \[CORRECCIÓN DE ERRORES] Corregir el punto muerto en Zookeeper SD.
*   \[CORRECCIÓN DE ERRORES] Solucione problemas de búsqueda difusa en la finalización automática de la interfaz de usuario web.

## 1.5.3 / 2017-05-11

*   \[CORRECCIÓN DE ERRORES] Corregir una posible pérdida de memoria en el descubrimiento de servicios de Kubernetes

## 1.5.2 / 2017-02-10

*   \[CORRECCIÓN DE ERRORES] Corregir la corrupción de la serie en un caso especial de mantenimiento de la serie donde
    la relación mínima serie-archivo-contracción se activa.
*   \[CORRECCIÓN DE ERRORES] Corregir dos condiciones de pánico, ambas relacionadas con el procesamiento de una serie
    programado para ser puesto en cuarentena.
*   \[MEJORA] Binarios construidos con Go1.7.5.

## 1.5.1 / 2017-02-07

*   \[CORRECCIÓN DE ERRORES] No pierda series de memoria completamente persistentes durante el punto de control.
*   \[CORRECCIÓN DE ERRORES] Corrija el reetiquetado que falla intermitentemente.
*   \[CORRECCIÓN DE ERRORES] Hacer `-storage.local.series-file-shrink-ratio` trabajo.
*   \[CORRECCIÓN DE ERRORES] Eliminar la condición de carrera de TestLoop.

## 1.5.0 / 2017-01-23

*   \[CAMBIAR] Utilice el orden lexicográfico para ordenar las alertas por nombre.
*   \[CARACTERÍSTICA] Agregue el descubrimiento de Joyent Triton.
*   \[CARACTERÍSTICA] Agregue destinos de raspado y API de destinos de alertmanager.
*   \[CARACTERÍSTICA] Agregue varias métricas relacionadas con la persistencia.
*   \[CARACTERÍSTICA] Agregue varias métricas relacionadas con el motor de consultas.
*   \[CARACTERÍSTICA] Agregue la capacidad de limitar las muestras de raspado y las métricas relacionadas.
*   \[CARACTERÍSTICA] Agregue acciones de reetiquetado de labeldrop y labelkeep.
*   \[CARACTERÍSTICA] Mostrar el directorio de trabajo actual en la página de estado.
*   \[MEJORA] Utilice estrictamente ServiceAccount for en la configuración de clúster en Kubernetes.
*   \[MEJORA] Varias mejoras en el rendimiento y la administración de la memoria.
*   \[CORRECCIÓN DE ERRORES] Corrija la autenticación básica para alertmanagers configurados a través de flag.
*   \[CORRECCIÓN DE ERRORES] No entre en pánico al decodificar datos corruptos.
*   \[CORRECCIÓN DE ERRORES] Ignore los archivos de puntos en el directorio de datos.
*   \[CORRECCIÓN DE ERRORES] Anular errores de federación intermedios.

## 1.4.1 / 2016-11-28

*   \[CORRECCIÓN DE ERRORES] Corregir la detección del servicio Consul

## 1.4.0 / 2016-11-25

*   \[CARACTERÍSTICA] Permitir la configuración de Alertmanagers a través de la detección de servicios
*   \[CARACTERÍSTICA] Mostrar Alertmanagers usados en la página de tiempo de ejecución en la interfaz de usuario
*   \[CARACTERÍSTICA] Perfiles de compatibilidad en la configuración de detección de servicios de AWS EC2
*   \[MEJORA] Eliminar el registro duplicado de errores de cliente de Kubernetes
*   \[MEJORA] Agregar métricas sobre el descubrimiento de servicios de Kubernetes
*   \[CORRECCIÓN DE ERRORES] Actualizar las anotaciones de alerta en la reevaluación
*   \[CORRECCIÓN DE ERRORES] Corregir la exportación del modificador de grupo en consultas PromQL
*   \[CORRECCIÓN DE ERRORES] Elimine posibles interbloqueos en varias implementaciones de detección de servicios
*   \[CORRECCIÓN DE ERRORES] Utilice el módulo float64 adecuado en PromQL `%` operaciones binarias
*   \[CORRECCIÓN DE ERRORES] Corregir un error de bloqueo en la detección del servicio de Kubernetes

## 1.3.1 / 2016-11-04

Esta versión de corrección de errores extrae las correcciones de la versión 1.2.3.

*   \[CORRECCIÓN DE ERRORES] Maneje correctamente la entrada Regex vacía en la configuración de reetiquetado.
*   \[CORRECCIÓN DE ERRORES] MOD (`%`) el operador no entra en pánico con pequeños números de coma flotante.
*   \[CORRECCIÓN DE ERRORES] Se actualizó el proveedor de miekg / dns para detectar correcciones de errores ascendentes.
*   \[MEJORA] Informes de errores de DNS mejorados.

## 1.2.3 / 2016-11-04

Tenga en cuenta que esta versión es cronológicamente posterior a 1.3.0.

*   \[CORRECCIÓN DE ERRORES] Maneje correctamente la hora de finalización antes de la hora de inicio en las consultas de rango.
*   \[CORRECCIÓN DE ERRORES] Error en negativo `-storage.staleness-delta`
*   \[CORRECCIÓN DE ERRORES] Maneje correctamente la entrada Regex vacía en la configuración de reetiquetado.
*   \[CORRECCIÓN DE ERRORES] MOD (`%`) el operador no entra en pánico con pequeños números de coma flotante.
*   \[CORRECCIÓN DE ERRORES] Se actualizó el proveedor de miekg / dns para detectar correcciones de errores ascendentes.
*   \[MEJORA] Informes de errores de DNS mejorados.

## 1.3.0 / 2016-11-01

Este es un cambio radical en el descubrimiento del servicio Kubernetes.

*   \[CAMBIAR] Reelaborar Kubernetes SD.
*   \[CARACTERÍSTICA] Agregar compatibilidad con la interpolación `target_label`.
*   \[CARACTERÍSTICA] Agregue metadatos GCE como metaetiquetas de Prometheus.
*   \[MEJORA] Agregue métricas de EC2 SD.
*   \[MEJORA] Agregar métricas de Azure SD.
*   \[MEJORA] Agregar búsqueda difusa a `/graph` textarea.
*   \[MEJORA] Mostrar siempre las etiquetas de instancia en la página de destino.
*   \[CORRECCIÓN DE ERRORES] La hora de finalización de la consulta de validación no es anterior a la hora de inicio.
*   \[CORRECCIÓN DE ERRORES] Error en negativo `-storage.staleness-delta`

## 1.2.2 / 2016-10-30

*   \[CORRECCIÓN DE ERRORES] Manejar correctamente on() en las alertas.
*   \[CORRECCIÓN DE ERRORES] UI: Trate correctamente las solicitudes abortadas.
*   \[CORRECCIÓN DE ERRORES] INTERFAZ DE USUARIO: Decodifique los parámetros de consulta de URL correctamente.
*   \[CORRECCIÓN DE ERRORES] Almacenamiento: Trate mejor la corrupción de datos (marcas de tiempo no monótonas).
*   \[CORRECCIÓN DE ERRORES] Almacenamiento remoto: Vuelva a agregar el indicador de tiempo de espera eliminado accidentalmente.
*   \[CORRECCIÓN DE ERRORES] Se han actualizado varios paquetes de proveedores para recoger las correcciones de errores ascendentes.

## 1.2.1 / 2016-10-10

*   \[CORRECCIÓN DE ERRORES] Contar los desalojos de fragmentos correctamente para que el servidor no lo haga
    supongamos que se queda sin memoria y posteriormente estrangula la ingestión.
*   \[CORRECCIÓN DE ERRORES] Usa Go1.7.1 para binarios prediseñados para solucionar problemas en MacOS Sierra.

## 1.2.0 / 2016-10-07

*   \[CARACTERÍSTICA] Codificación más limpia de parámetros de consulta en `/graph` Direcciones URL.
*   \[CARACTERÍSTICA] PromQL: Añadir `minute()` función.
*   \[CARACTERÍSTICA] Agregue la detección de servicios de GCE.
*   \[CARACTERÍSTICA] Permita cualquier cadena UTF-8 válida como nombre de trabajo.
*   \[CARACTERÍSTICA] Permitir la desactivación del almacenamiento local.
*   \[CARACTERÍSTICA] Detección de servicios EC2: Exponer `ec2_instance_state`.
*   \[MEJORA] Varias mejoras de rendimiento en el almacenamiento local.
*   \[CORRECCIÓN DE ERRORES] Detección de servicios de Zookeeper: elimine los nodos eliminados.
*   \[CORRECCIÓN DE ERRORES] Descubrimiento del servicio Zookeeper: estado de resincronización después de un error de Zookeeper.
*   \[CORRECCIÓN DE ERRORES] Elimine JSON del encabezado HTTP Accept.
*   \[CORRECCIÓN DE ERRORES] Corregir la validación del indicador de url de Alertmanager.
*   \[CORRECCIÓN DE ERRORES] Arreglar la condición de la carrera en el apagado.
*   \[CORRECCIÓN DE ERRORES] No falle el descubrimiento de Consul en la puesta en marcha de Prometheus cuando Consul
    está abajo.
*   \[CORRECCIÓN DE ERRORES] Manejar NaN en `changes()` correctamente.
*   \[CAMBIAR] **Experimental** ruta de escritura remota: elimine el uso de gRPC.
*   \[CAMBIAR] **Experimental** Ruta de escritura remota: Configuración a través del archivo de configuración
    en lugar de indicadores de línea de comandos.
*   \[CARACTERÍSTICA] **Experimental** ruta de escritura remota: agregue autenticación básica HTTP y TLS.
*   \[CARACTERÍSTICA] **Experimental** ruta de escritura remota: Soporte para reetiquetado.

## 1.1.3 / 2016-09-16

*   \[MEJORA] Utilice la imagen base de golang-builder para las pruebas en CircleCI.
*   \[MEJORA] Se agregaron pruebas unitarias para la federación.
*   \[CORRECCIÓN DE ERRORES] Desduplicar correctamente las familias métricas en la salida de la federación.

## 1.1.2 / 2016-09-08

*   \[CORRECCIÓN DE ERRORES] Permitir nombres de etiquetas que coincidan con palabras clave.

## 1.1.1 / 2016-09-07

*   \[CORRECCIÓN DE ERRORES] Corregir el escape de IPv6 en integraciones de detección de servicios
*   \[CORRECCIÓN DE ERRORES] Corregir la asignación predeterminada de puertos de raspado para IPv6

## 1.1.0 / 2016-09-03

*   \[CARACTERÍSTICA] Agregar `quantile` y `quantile_over_time`.
*   \[CARACTERÍSTICA] Agregar `stddev_over_time` y `stdvar_over_time`.
*   \[CARACTERÍSTICA] Agregue varias funciones de fecha y hora.
*   \[CARACTERÍSTICA] Añadido `toUpper` y `toLower` dar formato a plantillas.
*   \[CARACTERÍSTICA] Permitir el reetiquetado de alertas.
*   \[CARACTERÍSTICA] Permitir URL en destinos definidos a través de un archivo JSON.
*   \[CARACTERÍSTICA] Añadir función idelta.
*   \[CARACTERÍSTICA] Botón 'Eliminar gráfico' en la página /graph.
*   \[CARACTERÍSTICA] Kubernetes SD: agregue el nombre del nodo y la IP del host a la detección de pods.
*   \[CARACTERÍSTICA] Nueva ruta de escritura de almacenamiento remoto. ¡EXPERIMENTAL!
*   \[MEJORA] Mejore las búsquedas de índices de series temporales.
*   \[MEJORA] Prohibir configuraciones de reetiquetado no válidas.
*   \[MEJORA] Se han mejorado varias pruebas.
*   \[MEJORA] Agregue la métrica de recuperación de bloqueos 'started_dirty'.
*   \[MEJORA] Corrija (y simplifique) los iteradores de serie de relleno.
*   \[MEJORA] Agregar enlace de trabajo en la página de destino.
*   \[MEJORA] Mensaje en la página Alertas vacía.
*   \[MEJORA] Varias refactorizaciones y limpiezas de código interno.
*   \[MEJORA] Varias mejoras en el sistema de construcción.
*   \[CORRECCIÓN DE ERRORES] Detecte errores al desmarcar fragmentos codificados delta/doubleDelta.
*   \[CORRECCIÓN DE ERRORES] Corregir la carrera de datos en la prueba de lexer y lexer.
*   \[CORRECCIÓN DE ERRORES] Recorte el espacio en blanco extraviado del archivo de token del portador.
*   \[CORRECCIÓN DE ERRORES] Evite el pánico de dividir por cero en query_range?step=0.
*   \[CORRECCIÓN DE ERRORES] Detectar archivos de reglas no válidos en el inicio.
*   \[CORRECCIÓN DE ERRORES] Corregir el tratamiento de restablecimiento del contador en PromQL.
*   \[CORRECCIÓN DE ERRORES] Solucione los problemas de escape html de reglas.
*   \[CORRECCIÓN DE ERRORES] Elimine las etiquetas internas de las alertas enviadas a AM.

## 1.0.2 / 2016-08-24

*   \[CORRECCIÓN DE ERRORES] Limpie los destinos antiguos después de la recarga de la configuración.

## 1.0.1 / 2016-07-21

*   \[CORRECCIÓN DE ERRORES] Salga con error en los argumentos de línea de comandos que no marcan.
*   \[CORRECCIÓN DE ERRORES] Actualice las plantillas de consola de ejemplo a una nueva API HTTP.
*   \[CORRECCIÓN DE ERRORES] Vuelva a agregar indicadores de registro.

## 1.0.0 / 2016-07-18

*   \[CAMBIAR] Quitar palabras clave de lenguaje de consulta obsoletas
*   \[CAMBIAR] Cambiar Kubernetes SD para que requiera especificar el rol de Kubernetes
*   \[CAMBIAR] Utilice la dirección de servicio en Consul SD si está disponible
*   \[CAMBIAR] Estandarizar todas las métricas internas de Prometheus a segundas unidades
*   \[CAMBIAR] Quitar la API HTTP heredada sin versiones
*   \[CAMBIAR] Eliminar la ingesta heredada del formato de métrica JSON
*   \[CAMBIAR] Eliminar obsoleto `target_groups` configuración
*   \[CARACTERÍSTICA] Agregar operación de potencia binaria a PromQL
*   \[CARACTERÍSTICA] Agregar `count_values` agregador
*   \[CARACTERÍSTICA] Agregar `-web.route-prefix` bandera
*   \[CARACTERÍSTICA] Conceder `on()`, `by()`, `without()` en PromQL con conjuntos de etiquetas vacíos
*   \[MEJORA] Hacer `topk/bottomk` agregadores de funciones de consulta
*   \[CORRECCIÓN DE ERRORES] Corregir anotaciones en la impresión de reglas de alerta
*   \[CORRECCIÓN DE ERRORES] Expandir plantillas de alerta en el momento de la evaluación
*   \[CORRECCIÓN DE ERRORES] Arreglar el manejo de casos de borde en la recuperación de fallas
*   \[CORRECCIÓN DE ERRORES] Ocultar los indicadores del paquete de prueba de la salida de ayuda

## 0.20.0 / 2016-06-15

Esta versión contiene varios cambios importantes en el esquema de configuración.

*   \[CARACTERÍSTICA] Permitir la configuración de varios Alertmanagers
*   \[CARACTERÍSTICA] Agregar el nombre del servidor a la configuración de TLS
*   \[CARACTERÍSTICA] Agregue etiquetas para todas las direcciones de nodo y descubra el puerto de nodo si está disponible en Kubernetes SD
*   \[MEJORA] Errores de configuración más significativos
*   \[MEJORA] Marcas de tiempo de raspado redondas a milisegundos en la interfaz de usuario web
*   \[MEJORA] Hacer que el número de bloqueos de huellas dactilares de almacenamiento sea configurable
*   \[CORRECCIÓN DE ERRORES] Corregir el análisis de fecha en gráficos de plantillas de consola
*   \[CORRECCIÓN DE ERRORES] Corregir archivos de consola estáticos en imágenes de Docker
*   \[CORRECCIÓN DE ERRORES] Corregir solicitudes JS XHR de consola para IE11
*   \[CORRECCIÓN DE ERRORES] Agregar prefijo de ruta que falta en la nueva página de estado
*   \[CAMBIAR] Rebautizar `target_groups` Para `static_configs` en archivos de configuración
*   \[CAMBIAR] Rebautizar `names` Para `files` en la configuración SD del archivo
*   \[CAMBIAR] Eliminar la opción de configuración del puerto kubelet en la configuración SD de Kubernetes

## 0.19.3 / 2016-06-14

*   \[CORRECCIÓN DE ERRORES] Maneje aplicaciones de Marathon con cero puertos
*   \[CORRECCIÓN DE ERRORES] Solucionar el pánico de inicio en la capa de recuperación

## 0.19.2 / 2016-05-29

*   \[CORRECCIÓN DE ERRORES] Manejo correcto `GROUP_LEFT` y `GROUP_RIGHT` sin etiquetas en
    representación de cadenas de expresiones y en reglas.
*   \[CORRECCIÓN DE ERRORES] Uso `-web.external-url` para nuevos extremos de estado.

## 0.19.1 / 2016-05-25

*   \[CORRECCIÓN DE ERRORES] Manejar el pánico de detección de servicios que afecta a Kubernetes SD
*   \[CORRECCIÓN DE ERRORES] Solucionar el problema de visualización de la interfaz de usuario web en algunos navegadores

## 0.19.0 / 2016-05-24

Esta versión contiene un cambio importante en el lenguaje de consulta. Por favor, lea
la documentación sobre el comportamiento de agrupación de la coincidencia vectorial:

<https://prometheus.io/docs/querying/operators/#vector-matching>

*   \[CARACTERÍSTICA] Agregar detección experimental de servicios de Microsoft Azure
*   \[CARACTERÍSTICA] Agregar `ignoring` modificador para operaciones binarias
*   \[CARACTERÍSTICA] Agregar detección de pods a la detección de servicios de Kubernetes
*   \[CAMBIAR] La coincidencia vectorial toma etiquetas de agrupación de un lado
*   \[MEJORA] Intervalo de tiempo de soporte en /api/v1/series endpoint
*   \[MEJORA] Página de estado de partición en páginas individuales
*   \[CORRECCIÓN DE ERRORES] Solucionar el problema de colgar rasguños de objetivo

## 0.18.0 / 2016-04-18

*   \[CORRECCIÓN DE ERRORES] Fijar la precedencia del operador en PromQL
*   \[CORRECCIÓN DE ERRORES] Nunca deje caer el trozo de cabeza abierto
*   \[CORRECCIÓN DE ERRORES] Corregir la falta de 'keep_common' al imprimir el nodo AST
*   \[CAMBIO/CORRECCIÓN DE ERRORES] La identidad de destino considera la ruta de acceso y los parámetros adicionalmente al host y al puerto
*   \[CAMBIAR] Cambiar nombre de métrica `prometheus_local_storage_invalid_preload_requests_total` Para `prometheus_local_storage_non_existent_series_matches_total`
*   \[CAMBIAR] Se ha eliminado la compatibilidad con la sintaxis de la regla de alerta antigua
*   \[CARACTERÍSTICA] Destinos de deduplicado dentro del mismo trabajo de raspado
*   \[CARACTERÍSTICA] Agregue codificación de fragmentos varbit (mayor compresión, más uso de CPU, deshabilitado de forma predeterminada)
*   \[CARACTERÍSTICA] Agregar `holt_winters` función de consulta
*   \[CARACTERÍSTICA] Añadir complemento relativo `unless` operador a PromQL
*   \[MEJORA] Poner en cuarentena el archivo de la serie si se producen daños en los datos (en lugar de bloquearse)
*   \[MEJORA] Validar url de Alertmanager
*   \[MEJORA] Usar UTC para la marca de tiempo de compilación
*   \[MEJORA] Mejorar el rendimiento de las consultas de índice (especialmente para series temporales activas)
*   \[MEJORA] Duración de la recarga de la configuración del instrumento
*   \[MEJORA] Capa de recuperación de instrumentos
*   \[MEJORA] Agregar versión de Go a `prometheus_build_info` métrico

## 0.17.0 / 2016-03-02

¡Esta versión ya no funciona con Alertmanager 0.0.4 y versiones anteriores!
La sintaxis de la regla de alerta también ha cambiado, pero se admite la sintaxis anterior
hasta la versión 0.18.

Todas las expresiones regulares en PromQL están ancladas ahora, coincidiendo con el comportamiento de
expresiones regulares en archivos de configuración.

*   \[CAMBIAR] Integración con Alertmanager 0.1.0 y versiones posteriores
*   \[CAMBIAR] Modo de almacenamiento degradado renombrado a modo apresurado
*   \[CAMBIAR] Nueva sintaxis de regla de alerta
*   \[CAMBIAR] Agregar validación de etiquetas en la ingestión
*   \[CAMBIAR] Los emparejadores de expresiones regulares en PromQL están anclados
*   \[CARACTERÍSTICA] Agregar `without` modificador de agregación
*   \[CARACTERÍSTICA] Enviar notificaciones de alertas resueltas a Alertmanager
*   \[CARACTERÍSTICA] Permitir una precisión de milisegundos en el archivo de configuración
*   \[CARACTERÍSTICA] Apoye Smartstack Nerve de AirBnB para el descubrimiento de servicios
*   \[MEJORA] El almacenamiento cambia con menos frecuencia entre el modo regular y el apresurado.
*   \[MEJORA] El almacenamiento cambia al modo apresurado si hay demasiados trozos de memoria.
*   \[MEJORA] Se ha añadido más instrumentación de almacenamiento
*   \[MEJORA] Instrumentación mejorada del controlador de notificaciones
*   \[CORRECCIÓN DE ERRORES] No cuente los trozos de cabeza como trozos que esperan la persistencia
*   \[CORRECCIÓN DE ERRORES] Controlar correctamente las solicitudes HTTP options a la API
*   \[CORRECCIÓN DE ERRORES] Análisis de rangos en PromQL fijo
*   \[CORRECCIÓN DE ERRORES] Validar correctamente los parámetros del indicador de URL
*   \[CORRECCIÓN DE ERRORES] Errores de análisis de argumentos de registro
*   \[CORRECCIÓN DE ERRORES] Manejar correctamente la creación de destino con una configuración TLS incorrecta
*   \[CORRECCIÓN DE ERRORES] Solución del problema de sincronización del punto de control

## 0.16.2 / 2016-01-18

*   \[CARACTERÍSTICA] Se agregaron múltiples opciones de autenticación para la detección de EC2
*   \[CARACTERÍSTICA] Se agregaron varias etiquetas meta para el descubrimiento de EC2
*   \[CARACTERÍSTICA] Permitir URL completas en grupos objetivo estáticos (utilizados, por ejemplo, por el `blackbox_exporter`)
*   \[CARACTERÍSTICA] Agregar integración de almacenamiento remoto de Graphite
*   \[CARACTERÍSTICA] Crear destinos de Kubernetes independientes para los servicios y sus endpoints
*   \[CARACTERÍSTICA] Agregar `clamp_{min,max}` funciones para PromQL
*   \[CARACTERÍSTICA] El parámetro de tiempo omitido en la consulta de API tiene como valor predeterminado ahora
*   \[MEJORA] Truncamiento de archivos de series temporales menos frecuente
*   \[MEJORA] Número de instrumento de series temporales eliminadas manualmente
*   \[MEJORA] Ignorar el directorio perdido+encontrado durante la detección de la versión de almacenamiento
*   \[CAMBIAR] Kubernetes `masters` renombrado a `api_servers`
*   \[CAMBIAR] Los objetivos "saludables" y "no saludables" ahora se denominan "arriba" y "abajo" en la interfaz de usuario web
*   \[CAMBIAR] Eliminar el 2º argumento no documentado del `delta` función.
    (Este es un CAMBIO DE RUPTURA para los usuarios del 2º argumento indocumentado).
*   \[CORRECCIÓN DE ERRORES] Devolver códigos de estado HTTP adecuados en errores de API
*   \[CORRECCIÓN DE ERRORES] Corregir la configuración de autenticación de Kubernetes
*   \[CORRECCIÓN DE ERRORES] Corregir el OFFSET eliminado de la evaluación y visualización de reglas
*   \[CORRECCIÓN DE ERRORES] No se bloquee al fallar la inicialización de Consul SD
*   \[CORRECCIÓN DE ERRORES] Revertir los cambios en el autocompletado de métricas
*   \[CORRECCIÓN DE ERRORES] Agregar validación de desbordamiento de configuración para la configuración de TLS
*   \[CORRECCIÓN DE ERRORES] Omitir nodos de Zookeeper ya vistos en el conjunto de servidores SD
*   \[CORRECCIÓN DE ERRORES] No federar muestras obsoletas
*   \[CORRECCIÓN DE ERRORES] Mover NaN al final del resultado para `topk/bottomk/sort/sort_desc/min/max`
*   \[CORRECCIÓN DE ERRORES] Limitar la extrapolación de `delta/rate/increase`
*   \[CORRECCIÓN DE ERRORES] Corregir errores no controlados en la evaluación de reglas

Algunos cambios en el descubrimiento del servicio kubernetes fueron la integración desde entonces
fue lanzado como una función beta.

## 0.16.1 / 2015-10-16

*   \[CARACTERÍSTICA] Agregar `irate()` función.
*   \[MEJORA] Se ha mejorado la finalización automática en el explorador de expresiones.
*   \[CAMBIAR] Kubernetes SD mueve la etiqueta de nodo a la etiqueta de instancia.
*   \[CORRECCIÓN DE ERRORES] Escape regexes en plantillas de consola.

## 0.16.0 / 2015-10-09

CAMBIOS DE RUPTURA:

*   Los tarballs de lanzamiento ahora contienen los binarios compilados en un directorio anidado.
*   El `hash_mod` La acción de reetiquetado ahora utiliza hashes MD5 en lugar de hashes FNV para
    lograr una mejor distribución.
*   La metaetiqueta DNS-SD `__meta_dns_srv_name` se cambió el nombre a `__meta_dns_name`
    Para reflejar la compatibilidad con tipos de registros DNS distintos de `SRV`.
*   El intervalo de actualización completa predeterminado para la detección de servicios basados en archivos ha sido
    aumentó de 30 segundos a 5 minutos.
*   En el reetiquetado, partes de una etiqueta de origen que no coincidían con
    La expresión regular especificada ya no se incluye en el reemplazo
    salida.
*   Las consultas ya no se interpolan entre dos puntos de datos. En cambio, el resultado
    value siempre será el valor más reciente antes de la marca de tiempo de la consulta de evaluación.
*   Las expresiones regulares suministradas a través de la configuración ahora están ancladas para que coincidan
    cadenas completas en lugar de subcadenas.
*   Las etiquetas globales ya no se agregan al almacenar series temporales. En lugar de
    Sólo se anexan cuando se comunican con sistemas externos
    (Alertmanager, almacenamientos remotos, federación). Por lo tanto, también han sido renombrados.
    De `global.labels` Para `global.external_labels`.
*   Los nombres y unidades de las métricas relacionadas con los apéndices de ejemplo de almacenamiento remoto tienen
    se ha cambiado.
*   El soporte experimental para escribir en InfluxDB se ha actualizado para que funcione
    con InfluxDB 0.9.x. 0.8.x las versiones de InfluxDB ya no son compatibles.
*   Secuencias de escape en literales de cadena de comillas dobles y simples en reglas o consultas
    Las expresiones ahora se interpretan como secuencias de escape en literales de cadena Go
    (<https://golang.org/ref/spec#String_literals>).

Futuros cambios de ruptura / características obsoletas:

*   El `delta()` La función tenía un segundo argumento booleano opcional no documentado
    para que se comporte como `increase()`. Este segundo argumento se eliminará en
    el futuro. Migrar cualquier ocurrencia de `delta(x, 1)` utilizar `increase(x)`
    en lugar de.
*   Soporte para operadores de filtro entre dos valores escalares (como `2 > 1`) será
    eliminado en el futuro. Estos requerirán un `bool` modificador en el operador,
    p. ej..  `2 > bool 1`.

Todos los cambios:

*   \[CAMBIAR] Retitulado `global.labels` Para `global.external_labels`.
*   \[CAMBIAR] La venta ahora se realiza a través de govendor en lugar de godep.
*   \[CAMBIAR] Cambiar la página raíz de la interfaz de usuario web para mostrar la interfaz gráfica en lugar de
    la página de estado del servidor.
*   \[CAMBIAR] Anexar etiquetas globales solo cuando se comunique con sistemas externos
    en lugar de almacenarlos localmente.
*   \[CAMBIAR] Cambiar todos los regexes de la configuración para hacer coincidencias de cadena completa
    en lugar de coincidencias de subcadenas.
*   \[CAMBIAR] Elimine la interpolación de valores vectoriales en las consultas.
*   \[CAMBIAR] Para alerta `SUMMARY`/`DESCRIPTION` campos de plantilla, transmitir la alerta
    valor a `float64` para trabajar con funciones de plantillas comunes.
*   \[CAMBIAR] En el reetiquetado, no incluya piezas de etiquetas de origen inigualables en el
    reemplazo.
*   \[CAMBIAR] Cambiar el intervalo de actualización completa predeterminado para el servicio basado en archivos
    descubrimiento de 30 segundos a 5 minutos.
*   \[CAMBIAR] Cambiar el nombre de la metaetiqueta DNS-SD `__meta_dns_srv_name` Para
    `__meta_dns_name` para reflejar la compatibilidad con otros tipos de registros que no sean `SRV`.
*   \[CAMBIAR] Los tarballs de lanzamiento ahora contienen los binarios en un directorio anidado.
*   \[CAMBIAR] Actualice la compatibilidad de escritura de InfluxDB para que funcione con InfluxDB 0.9.x.
*   \[CARACTERÍSTICA] Admite secuencias de escape completas "estilo Go" en cadenas y agrega raw
    literales de cadena.
*   \[CARACTERÍSTICA] Agregue compatibilidad con la detección de servicios de EC2.
*   \[CARACTERÍSTICA] Permitir la configuración de opciones de TLS en configuraciones de raspado.
*   \[CARACTERÍSTICA] Agregue instrumentación en torno a las recargas de configuración.
*   \[CARACTERÍSTICA] Agregar `bool` modificador de los operadores de comparación para habilitar booleanos
    (`0`/`1`) salida en lugar de filtrado.
*   \[CARACTERÍSTICA] En la detección de conjuntos de servidores de Zookeeper, proporcione `__meta_serverset_shard`
    con el número de fragmento del conjunto de servidores.
*   \[CARACTERÍSTICA] Proporcionar `__meta_consul_service_id` meta etiqueta en el servicio Consul
    descubrimiento.
*   \[CARACTERÍSTICA] Permitir expresiones escalares en reglas de grabación para habilitar casos de uso
    como la creación de métricas constantes.
*   \[CARACTERÍSTICA] Agregar `label_replace()` y `vector()` funciones del lenguaje de consulta.
*   \[CARACTERÍSTICA] En Descubrimiento del servicio de Cónsul, rellene el `__meta_consul_dc`
    etiqueta de centro de datos del agente de Consul cuando no está configurada en la SD de Consul
    config.
*   \[CARACTERÍSTICA] Raspar todos los servicios en la lista de servicios vacía en el servicio Consul
    descubrimiento.
*   \[CARACTERÍSTICA] Agregar `labelmap` Acción de reetiquetado para asignar un conjunto de etiquetas de entrada a un
    conjunto de etiquetas de salida utilizando expresiones regulares.
*   \[CARACTERÍSTICA] Introducir `__tmp` como un prefijo de etiqueta de reetiquetado que está garantizado
    para no ser utilizado por Prometeo internamente.
*   \[CARACTERÍSTICA] Descubrimiento de servicios basado en Kubernetes.
*   \[CARACTERÍSTICA] Descubrimiento de servicios basado en maratón.
*   \[CARACTERÍSTICA] Admite varios nombres de series en la biblioteca JavaScript de gráficos de consola.
*   \[CARACTERÍSTICA] Permitir la recarga de la configuración a través del controlador web en `/-/reload`.
*   \[CARACTERÍSTICA] Actualizaciones de promtool para reflejar la nueva configuración de Prometheus
    Funciones.
*   \[CARACTERÍSTICA] Agregar `proxy_url` para raspar configuraciones para habilitar el uso de
    servidores proxy.
*   \[CARACTERÍSTICA] Añade plantillas de consola para el propio Prometheus.
*   \[CARACTERÍSTICA] Permitir reetiquetar el esquema de protocolo de los objetivos.
*   \[CARACTERÍSTICA] Agregar `predict_linear()` función de lenguaje de consulta.
*   \[CARACTERÍSTICA] Compatibilidad con la autenticación mediante tokens de portador, certificados de cliente y
    Certificados de CA.
*   \[CARACTERÍSTICA] Implementar expresiones unarias para tipos vectoriales (`-foo`, `+foo`).
*   \[CARACTERÍSTICA] Agregue plantillas de consola para el exportador SNMP.
*   \[CARACTERÍSTICA] Permitir el reetiquetado de los parámetros de consulta de raspado de destino.
*   \[CARACTERÍSTICA] Agregar soporte para `A` y `AAAA` registros en la detección de servicios DNS.
*   \[MEJORA] Arregle varias pruebas escamosas.
*   \[MEJORA] Cambie al paquete de enrutamiento común.
*   \[MEJORA] Utilice un decodificador métrico más resistente.
*   \[MEJORA] Actualizar las dependencias de proveedor.
*   \[MEJORA] Agregue compresión a más controladores HTTP.
*   \[MEJORA] Haga que la cadena de ayuda del indicador -web.external-url sea más detallada.
*   \[MEJORA] Mejore las métricas en torno a las colas de almacenamiento remoto.
*   \[MEJORA] Utilice Go 1.5.1 en lugar de Go 1.4.2 en las compilaciones.
*   \[MEJORA] Actualizar el diagrama de arquitectura en el `README.md`.
*   \[MEJORA] Se anexa un ejemplo de tiempo de espera en la capa de recuperación si el almacenamiento es
    atraso.
*   \[MEJORA] Hacer `hash_mod` la acción de reetiquetado utiliza MD5 en lugar de FNV para
    permitir una mejor distribución de hash.
*   \[MEJORA] Mejor seguimiento de los objetivos entre el mismo descubrimiento de servicios
    mecanismos en una configuración de raspado.
*   \[MEJORA] Manejar el analizador y el tiempo de ejecución de evaluación de consultas entra en pánico más
    agraciadamente.
*   \[MEJORA] Agregue IDENTIFICADORes a las etiquetas H2 en la página de estado para permitir la vinculación anclada.
*   \[CORRECCIÓN DE ERRORES] Corrija la visualización de varias rutas con la detección de conjuntos de servidores de Zookeeper.
*   \[CORRECCIÓN DE ERRORES] Corrija el alto uso de la CPU en la recarga de la configuración.
*   \[CORRECCIÓN DE ERRORES] Corrección que desaparece `__params` en la recarga de la configuración.
*   \[CORRECCIÓN DE ERRORES] Hacer `labelmap` acción disponible a través de la configuración.
*   \[CORRECCIÓN DE ERRORES] Arreglar el acceso directo de los campos protobuf.
*   \[CORRECCIÓN DE ERRORES] Solucione el pánico en el error de solicitud de Consul.
*   \[CORRECCIÓN DE ERRORES] Redirección del extremo del gráfico para configuraciones prefijadas.
*   \[CORRECCIÓN DE ERRORES] Corrija el comportamiento de eliminación de archivos de series al purgar series archivadas.
*   \[CORRECCIÓN DE ERRORES] Corrija la comprobación de errores y el inicio de sesión alrededor de los puntos de control.
*   \[CORRECCIÓN DE ERRORES] Corregir la inicialización del mapa en el administrador de destino.
*   \[CORRECCIÓN DE ERRORES] Corrija el drenaje de eventos del observador de archivos en la detección de servicios basados en archivos.
*   \[CORRECCIÓN DE ERRORES] Agregar `POST` manejador para `/debug` endpoints para corregir la generación de perfiles de CPU.
*   \[CORRECCIÓN DE ERRORES] Arregle varias pruebas escamosas.
*   \[CORRECCIÓN DE ERRORES] Arreglar el busylooping en caso de que una configuración de scrape no tenga destino
    proveedores definidos.
*   \[CORRECCIÓN DE ERRORES] Corregir el comportamiento de salida del proveedor de destino estático.
*   \[CORRECCIÓN DE ERRORES] Corrija el bucle de recarga de la configuración al apagarse.
*   \[CORRECCIÓN DE ERRORES] Agregue la comprobación que falta para la expresión nula en el analizador de expresiones.
*   \[CORRECCIÓN DE ERRORES] Corregir errores al manejar errores en el código de prueba.
*   \[CORRECCIÓN DE ERRORES] Corrija la metaetiqueta del puerto Consul.
*   \[CORRECCIÓN DE ERRORES] Corrija el error del lexer que trataba los dígitos Unicode no latinos como dígitos.
*   \[LIMPIEZA] Quite el ejemplo de federación obsoleto de las plantillas de consola.
*   \[LIMPIEZA] Elimine la inclusión duplicada de Bootstrap JS en la página del gráfico.
*   \[LIMPIEZA] Cambie al paquete de registro común.
*   \[LIMPIEZA] Actualice los scripts del entorno de compilación y Makefiles para que funcionen mejor con
    mecanismos de compilación de Go nativos y nuevo soporte de proveedores experimentales de Go 1.5.
*   \[LIMPIEZA] Elimine el aviso registrado sobre el cambio de formato de archivo de configuración 0.14.x.
*   \[LIMPIEZA] Mueva la modificación de la etiqueta de métrica de tiempo de raspado a SampleAppenders.
*   \[LIMPIEZA] Cambiar de `github.com/client_golang/model` Para
    `github.com/common/model` y limpiezas de tipos relacionadas.
*   \[LIMPIEZA] Cambiar de `github.com/client_golang/extraction` Para
    `github.com/common/expfmt` y limpiezas de tipos relacionadas.
*   \[LIMPIEZA] Salga de Prometheus cuando el servidor web encuentre un error de inicio.
*   \[LIMPIEZA] Elimine los enlaces silenciadores de alertas no funcionales en la página de alertas.
*   \[LIMPIEZA] Limpiezas generales de comentarios y código, derivados de `golint`,
    `go vet`, o de otro modo.
*   \[LIMPIEZA] Al ingresar a la recuperación de bloqueos, dígales a los usuarios cómo apagarlos limpiamente
    Prometeo.
*   \[LIMPIEZA] Quite la compatibilidad interna con consultas de varias instrucciones en el motor de consultas.
*   \[LIMPIEZA] Actualización AUTHORS.md.
*   \[LIMPIEZA] No avise/incremente la métrica al encontrar marcas de tiempo iguales para
    la misma serie al anexar.
*   \[LIMPIEZA] Resuelva las rutas relativas durante la carga de la configuración.

## 0.15.1 / 2015-07-27

*   \[CORRECCIÓN DE ERRORES] Corregir el comportamiento de coincidencia de vectores cuando hay una mezcla de igualdad y
    los emparejadores que no son de igualdad en un selector vectorial y un emparejador no coinciden con ninguna serie.
*   \[MEJORA] Permitir anulación `GOARCH` y `GOOS` en Makefile.INCLUDE.
*   \[MEJORA] Actualizar las dependencias de proveedor.

## 0.15.0 / 2015-07-21

CAMBIOS DE RUPTURA:

*   Las rutas de acceso relativas para los archivos de reglas ahora se evalúan en relación con el archivo de configuración.
*   Indicadores de accesibilidad externa (`-web.*`) consolidado.
*   El directorio de almacenamiento predeterminado se ha cambiado de `/tmp/metrics`
    Para `data` en el directorio local.
*   El `rule_checker` la herramienta ha sido reemplazada por `promtool` con
    diferentes banderas y más funcionalidad.
*   Las etiquetas vacías ahora se eliminan al ingerirlas en el
    almacenamiento. Hacer coincidir etiquetas vacías ahora es equivalente a hacer coincidir sin establecer
    etiquetas (`mymetric{label=""}` ahora coincide con series que no tienen
    `label` conjunto en absoluto).
*   El especial `__meta_consul_tags` etiqueta en la detección del servicio Consul
    ahora comienza y termina con separadores de etiquetas para habilitar Regex más fácil
    cotejo.
*   El intervalo de raspado predeterminado se ha cambiado de 1 minuto a
    10 segundos.

Todos los cambios:

*   \[CAMBIAR] Cambiar el directorio de almacenamiento predeterminado a `data` en el actual
    directorio de trabajo.
*   \[CAMBIAR] Consolidar indicadores de accesibilidad externos (`-web.*`)en uno.
*   \[CAMBIAR] Desaprobar `keeping_extra` palabra clave modificadora, cámbiele el nombre a
    `keep_common`.
*   \[CAMBIAR] Mejore el rendimiento de la coincidencia de etiquetas y trate las etiquetas no establecidas
    como etiquetas vacías en los emparejadores de etiquetas.
*   \[CAMBIAR] Eliminar `rule_checker` y agregar genérico `promtool` CLI
    herramienta que permite comprobar reglas y archivos de configuración.
*   \[CAMBIAR] Resolver archivos de reglas relativos al archivo de configuración.
*   \[CAMBIAR] Restaure ScrapeInterval predeterminado de 1 minuto en lugar de 10 segundos.
*   \[CAMBIAR] Rodear `__meta_consul_tags` con separadores de etiquetas.
*   \[CAMBIAR] Actualice la consola de disco de nodo para nuevas etiquetas de sistema de archivos.
*   \[CARACTERÍSTICA] Añadir Cónsul `ServiceAddress`, `Address`y `ServicePort` como
    metaetiquetas para habilitar la configuración de una dirección de raspado personalizada si es necesario.
*   \[CARACTERÍSTICA] Agregar `hashmod` acción de reetiquetado para permitir horizontal
    fragmentación de servidores Prometheus.
*   \[CARACTERÍSTICA] Agregar `honor_labels` raspar la opción de configuración para no
    sobrescribir las etiquetas expuestas por el objetivo.
*   \[CARACTERÍSTICA] Agregar compatibilidad básica con la federación en `/federate`.
*   \[CARACTERÍSTICA] Agregar opcional `RUNBOOK` para alertar instrucciones.
*   \[CARACTERÍSTICA] Agregue etiquetas de destino de reetiquetado previamente a la página de estado.
*   \[CARACTERÍSTICA] Agregar extremo de información de versión en `/version`.
*   \[CARACTERÍSTICA] Se agregó la versión 1 de la API estable inicial en `/api/v1`,
    incluyendo la capacidad de eliminar series y consultar más metadatos.
*   \[CARACTERÍSTICA] Permitir la configuración de parámetros de consulta al raspar extremos de métricas.
*   \[CARACTERÍSTICA] Permite eliminar series temporales a través de la nueva API v1.
*   \[CARACTERÍSTICA] Permitir que se vuelvan a etiquetar las métricas individuales ingeridas.
*   \[CARACTERÍSTICA] Permitir la carga de archivos de reglas desde un directorio completo.
*   \[CARACTERÍSTICA] Permitir expresiones escalares en consultas de rango, mejorar los mensajes de error.
*   \[CARACTERÍSTICA] Admite conjuntos de servidores de Zookeeper como mecanismo de detección de servicios.
*   \[MEJORA] Agregue circleci yaml para la compilación de prueba de Dockerfile.
*   \[MEJORA] Muestre siempre el rango de gráficos seleccionado, independientemente de los datos disponibles.
*   \[MEJORA] Cambie el campo de entrada de expresión a un área de texto de varias líneas.
*   \[MEJORA] Aplicar la estricta monotonicidad de las marcas de tiempo dentro de una serie.
*   \[MEJORA] Exportar información de compilación como métrica.
*   \[MEJORA] Mejorar la interfaz de usuario de `/alerts` página.
*   \[MEJORA] Mejore la visualización de las etiquetas de destino en la página de estado.
*   \[MEJORA] Mejore la funcionalidad de inicialización y enrutamiento del servicio web.
*   \[MEJORA] Mejore el manejo y la visualización de la URL de destino.
*   \[MEJORA] Nuevo dockerfile usando la imagen base alpine-glibc y la marca.
*   \[MEJORA] Otras correcciones menores.
*   \[MEJORA] Conserve el estado de alerta en todas las recargas.
*   \[MEJORA] Prettify flag ayuda a la salida aún más.
*   \[MEJORA] README.md actualizaciones.
*   \[MEJORA] Generar error en parámetros de configuración desconocidos.
*   \[MEJORA] Refinar la salida de la API HTTP v1.
*   \[MEJORA] Mostrar el contenido del archivo de configuración original en el estado
    en lugar de YAML serializado.
*   \[MEJORA] Inicie el controlador de señal HUP antes para no salir al HUP
    durante el inicio.
*   \[MEJORA] Se actualizaron las dependencias de proveedores.
*   \[CORRECCIÓN DE ERRORES] No entres en pánico `StringToDuration()` en una unidad de duración incorrecta.
*   \[CORRECCIÓN DE ERRORES] Salga en archivos de reglas no válidos al iniciar.
*   \[CORRECCIÓN DE ERRORES] Corregir una regresión en el `.Path` variable de plantilla de consola.
*   \[CORRECCIÓN DE ERRORES] Corregir la carga del descriptor de fragmentos.
*   \[CORRECCIÓN DE ERRORES] Arreglar consolas "Prometheus" enlace para apuntar a /
*   \[CORRECCIÓN DE ERRORES] Corregir casos de archivos de configuración vacíos
*   \[CORRECCIÓN DE ERRORES] Corregir las conversiones float to int en la codificación de fragmentos, que fueron
    roto para algunas arquitecturas.
*   \[CORRECCIÓN DE ERRORES] Corregir la detección de desbordamiento para la configuración del conjunto de servidores.
*   \[CORRECCIÓN DE ERRORES] Arreglar las condiciones de carrera en la capa de recuperación.
*   \[CORRECCIÓN DE ERRORES] Solucione el interbloqueo de apagado en el código SD de Consul.
*   \[CORRECCIÓN DE ERRORES] Corrija los objetivos de condición de carrera en makefile.
*   \[CORRECCIÓN DE ERRORES] Corregir error de visualización de valores en la consola web.
*   \[CORRECCIÓN DE ERRORES] Ocultar credenciales de autenticación en la configuración `String()` salida.
*   \[CORRECCIÓN DE ERRORES] Incrementar la métrica de contador sucio en el almacenamiento solo si
    `setDirty(true)` se llama.
*   \[CORRECCIÓN DE ERRORES] Actualizar periódicamente los servicios en Consul para recuperarse de
    eventos faltantes.
*   \[CORRECCIÓN DE ERRORES] Evitar la sobrescritura de la configuración global predeterminada al cargar un
    configuración.
*   \[CORRECCIÓN DE ERRORES] Propiamente lex `\r` como espacio en blanco en el lenguaje de expresión.
*   \[CORRECCIÓN DE ERRORES] Valide los nombres de etiquetas en grupos de destino JSON.
*   \[CORRECCIÓN DE ERRORES] Valide la presencia del campo regex en configuraciones de reetiquetado.
*   \[LIMPIEZA] Limpie la inicialización de las colas de almacenamiento remoto.
*   \[LIMPIEZA] Arreglar `go vet` y `golint` Violaciones.
*   \[LIMPIEZA] Limpieza general de reglas y código de lenguaje de consulta.
*   \[LIMPIEZA] Mejore y simplifique los pasos de compilación de Dockerfile.
*   \[LIMPIEZA] Mejore y simplifique la infraestructura de compilación, use go-bindata
    para activos web. Permitir la construcción sin git.
*   \[LIMPIEZA] Mover todos los paquetes de utilidades a común `util` subdirectorio.
*   \[LIMPIEZA] Refactorice el principal, el manejo de banderas y el paquete web.
*   \[LIMPIEZA] Quitar métodos no utilizados de `Rule` interfaz.
*   \[LIMPIEZA] Simplifique el manejo de la configuración predeterminada.
*   \[LIMPIEZA] Cambie los tiempos legibles por humanos en la interfaz de usuario web a UTC.
*   \[LIMPIEZA] Uso `templates.TemplateExpander` para todas las plantillas de página.
*   \[LIMPIEZA] Utilice la nueva API HTTP v1 para realizar consultas y gráficos.

## 0.14.0 / 2015-06-01

*   \[CAMBIAR] El formato de configuración cambió y cambió a YAML.
    (Ver el [herramienta de migración](https://github.com/prometheus/migrate/releases).)
*   \[MEJORA] Rediseño del descubrimiento de destinos que preservan el estado.
*   \[MEJORA] Permite especificar el esquema de URL de raspado y la autenticación HTTP básica para destinos no estáticos.
*   \[CARACTERÍSTICA] Permita adjuntar etiquetas significativas a los objetivos a través del reetiquetado.
*   \[CARACTERÍSTICA] Recarga de reglas/configuración en tiempo de ejecución.
*   \[CARACTERÍSTICA] Detección de destinos a través de la observación de archivos.
*   \[CARACTERÍSTICA] Descubrimiento de objetivos a través de Consul.
*   \[MEJORA] Evaluación simplificada de operaciones binarias.
*   \[MEJORA] Inicialización de componentes más estable.
*   \[MEJORA] Se ha agregado un lenguaje de prueba de expresiones internas.
*   \[CORRECCIÓN DE ERRORES] Corrija los enlaces del gráfico con el prefijo de ruta.
*   \[MEJORA] Permitir la construcción desde la fuente sin git.
*   \[MEJORA] Mejore el rendimiento del iterador de almacenamiento.
*   \[MEJORA] Cambie el formato de salida de registro y las banderas.
*   \[CORRECCIÓN DE ERRORES] Corregir el error de alineación de memoria para sistemas de 32 bits.
*   \[MEJORA] Mejorar el comportamiento de redirección web.
*   \[MEJORA] Permitir la anulación del nombre de host predeterminado para las URL de Prometheus.
*   \[CORRECCIÓN DE ERRORES] Corregir la doble barra diagonal en la URL enviada a alertmanager.
*   \[CARACTERÍSTICA] Agregue la función de consulta resets() para contar los restablecimientos de contador.
*   \[CARACTERÍSTICA] Agregue la función de consulta changes() para contar el número de veces que un medidor cambió.
*   \[CARACTERÍSTICA] Agregue la función de consulta increase() para calcular el aumento de un contador.
*   \[MEJORA] Limite las muestras recuperables a la ventana de retención del almacenamiento.

## 0.13.4 / 2015-05-23

*   \[CORRECCIÓN DE ERRORES] Arregla una carrera mientras revisas las asignaciones de huellas dactilares.

## 0.13.3 / 2015-05-11

*   \[CORRECCIÓN DE ERRORES] Maneje las colisiones de huellas dactilares correctamente.
*   \[CAMBIAR] Los comentarios en el archivo de reglas deben comenzar con `#`. (Los indocumentados `//`
    y `/*...*/` los estilos de comentario ya no son compatibles).
*   \[MEJORA] Cambiar al analizador y la evaluación del lenguaje de expresión personalizado
    engine, que genera mejores mensajes de error, corrige algunos casos de borde de análisis,
    y permite otras mejoras futuras (como las siguientes).
*   \[MEJORA] Limite el número máximo de consultas simultáneas.
*   \[MEJORA] Finalice las consultas en ejecución durante el apagado.

## 0.13.2 / 2015-05-05

*   \[MANTENIMIENTO] Se actualizaron las dependencias de proveedores a sus versiones más recientes.
*   \[MANTENIMIENTO] Incluye plantillas de rule_checker y consola en el tarball de lanzamiento.
*   \[CORRECCIÓN DE ERRORES] Ordene NaN como el valor más bajo.
*   \[MEJORA] Agregue funciones de raíz cuadrada, stddev y stdvar.
*   \[CORRECCIÓN DE ERRORES] Use scrape_timeout para el tiempo de espera de raspado, no scrape_interval.
*   \[MEJORA] Mejore la carga de trozos y trozos, aumente el rendimiento cuando
    lectura desde el disco.
*   \[CORRECCIÓN DE ERRORES] Mostrar error correcto en una respuesta DNS incorrecta.

## 0.13.1 / 2015-04-09

*   \[CORRECCIÓN DE ERRORES] Trate correctamente las series de memoria con cero trozos en el mantenimiento de la serie.
*   \[MEJORA] Mejore aún más la legibilidad del texto de uso.

## 0.13.0 / 2015-04-08

*   \[MEJORA] Codificación de doble delta para trozos, ahorrando típicamente el 40% de
    espacio, tanto en RAM como en disco.
*   \[MEJORA] Rediseño de la cola de persistencia de fragmentos, aumentando el rendimiento
    en discos giratorios significativamente.
*   \[MEJORA] Rediseño de la ingestión de muestras, aumentando el rendimiento de la ingestión.
*   \[CARACTERÍSTICA] Se agregaron las funciones ln, log2, log10 y exp al lenguaje de consulta.
*   \[CARACTERÍSTICA] Soporte de escritura experimental para InfluxDB.
*   \[CARACTERÍSTICA] Permitir marcas de tiempo personalizadas en la API de consulta instantánea.
*   \[CARACTERÍSTICA] Prefijo de ruta configurable para que las URL admitan proxies.
*   \[MEJORA] Aumento de la usabilidad de la CLI rule_checker.
*   \[CAMBIAR] Mostrar valores de flotación especiales como huecos.
*   \[MEJORA] Hizo que la salida de uso fuera más legible.
*   \[MEJORA] Mayor resiliencia del almacenamiento contra la corrupción de datos.
*   \[MEJORA] Varias mejoras en torno a la codificación de fragmentos.
*   \[MEJORA] Formato más agradable de la tabla de estado de destino en /status.
*   \[CAMBIAR] Cambie el nombre de INALCANZABLE a INSALUBRE, VIVO a SALUDABLE.
*   \[CORRECCIÓN DE ERRORES] Barra diagonal final en la URL de alertmanager.
*   \[CORRECCIÓN DE ERRORES] Evite +InfYs y similares, solo muestre +Inf.
*   \[CORRECCIÓN DE ERRORES] Se ha corregido el escape de HTML en varios lugares.
*   \[CORRECCIÓN DE ERRORES] Se ha corregido el manejo de valores especiales en la división y el módulo de la consulta
    Idioma.
*   \[CORRECCIÓN DE ERRORES] Arreglar embed-static.sh.
*   \[LIMPIEZA] Se agregaron pruebas iniciales de la API HTTP.
*   \[CLEANUP] Varias limpiezas de código.
*   \[MANTENIMIENTO] Se actualizaron las dependencias de proveedores a sus versiones más recientes.

## 0.12.0 / 2015-03-04

*   \[CAMBIAR] Utilice client_golang v0.3.1. ESTO CAMBIA LA HUELLA DIGITAL E INVALIDA
    TODAS LAS HUELLAS DACTILARES PERSISTIDAS. Debe limpiar su almacenamiento para usar esto o
    versiones posteriores. Hay un protector de versión en su lugar que le impedirá
    ejecutar Prometheus con los datos almacenados de un Prometheus más antiguo.
*   \[CORRECCIÓN DE ERRORES] El cambio anterior corrige una debilidad en el algoritmo de huellas dactilares.
*   \[MEJORA] El cambio anterior hace que la toma de huellas dactilares sea más rápida y menos asignación
    intensivo.
*   \[CARACTERÍSTICA] Operador OR y opciones de coincidencia vectorial. Consulte los documentos para obtener más información.
*   \[MEJORA] Notación científica y valores especiales de flotación (Inf, NaN) ahora
    soportado por el lenguaje de expresión.
*   \[CAMBIAR] Dockerfile hace que Prometheus use el volumen de Docker para almacenar datos
    (en lugar de /tmp/metrics).
*   \[CAMBIAR] Makefile utiliza Go 1.4.2.

## 0.11.1 / 2015-02-27

*   \[CORRECCIÓN DE ERRORES] Vuelva a completar el mantenimiento de la serie. (Desde 0.9.0rc4,
    o confirmar 0851945, las series no se archivarían, los descriptores de fragmentos sí lo harían
    no ser desalojado, y los trozos de cabeza rancios nunca se cerrarían. Esto sucedió
    debido a la eliminación accidental de una línea que llama a un (bien probado :) función.
*   \[CORRECCIÓN DE ERRORES] No cuente dos veces los fragmentos de cabeza leídos desde el punto de control en el inicio.
    También corrige un error relacionado pero menos grave en el conteo de descriptores de fragmentos.
*   \[CORRECCIÓN DE ERRORES] Verifique la última vez en el trozo de cabeza para el tiempo de espera del trozo de cabeza, no primero.
*   \[CAMBIAR] Actualice el proveedor debido a los cambios de proveedores en client_golang.
*   \[LIMPIEZA] Limpiezas de código.
*   \[MEJORA] Limite el número de series "sucias" contadas durante el punto de control.

## 0.11.0 / 2015-02-23

*   \[CARACTERÍSTICA] Introduzca un nuevo histograma de tipo de métrica con agregación del lado del servidor.
*   \[CARACTERÍSTICA] Agregue el operador de desplazamiento.
*   \[CARACTERÍSTICA] Agregue funciones de piso, ceil y redondo.
*   \[CAMBIAR] Cambie los identificadores de instancia para que sean host:port.
*   \[CAMBIAR] La gestión de dependencias y la creación de proveedores cambiaron/mejoraron.
*   \[CAMBIAR] Marcar cambios de nombre para crear coherencia entre varios Prometheus
    Binarios.
*   \[CAMBIAR] Mostrar un número ilimitado de métricas en autocompletar.
*   \[CAMBIAR] Agregue el tiempo de espera de la consulta.
*   \[CAMBIAR] Quite las etiquetas en el contador de errores persistentes.
*   \[MEJORA] Varias mejoras de rendimiento para la ingestión de muestras.
*   \[MEJORA] Varias mejoras de Makefile.
*   \[MEJORA] Varias mejoras en la plantilla de consola, incluyendo
    prueba de concepto para la federación a través de plantillas de consola.
*   \[MEJORA] Solucione los fallos de JS del gráfico y simplifique el código de gráficos.
*   \[MEJORA] Disminuya drásticamente los recursos para la incrustación de archivos.
*   \[MEJORA] La recuperación de fallos guarda los datos perdidos de la serie en el directorio 'huérfano'.
*   \[CORRECCIÓN DE ERRORES] Corregir el cálculo de claves de agrupación de agregación.
*   \[CORRECCIÓN DE ERRORES] Corregir la ruta de descarga de Go para varias arquitecturas.
*   \[CORRECCIÓN DE ERRORES] Se ha corregido el vínculo de la imagen de estado de compilación de Travis.
*   \[CORRECCIÓN DE ERRORES] Corregir la falta de coincidencia de la versión de Rickshaw / D3.
*   \[LIMPIEZA] Varias limpiezas de código.

## 0.10.0 / 2015-01-26

*   \[CAMBIAR] Formato de resultado JSON más eficiente en la API de consulta. Esto requiere
    las versiones actualizadas de PromDash y prometheus_cli también.
*   \[MEJORA] Se excluyen los activos de Bootstrap no minificados y los mapas de Bootstrap
    de incrustar en el binario. Esos archivos solo se utilizan para la depuración,
    y, a continuación, puede utilizar -web.use-local-assets. Al incluir menos archivos, el
    El uso de RAM durante la compilación es mucho más manejable.
*   \[MEJORA] El vínculo de ayuda apunta a <https://prometheus.github.io> Ahora.
*   \[CARACTERÍSTICA] Consolas para haproxy y cloudwatch.
*   \[CORRECCIÓN DE ERRORES] Varias correcciones a gráficos en consolas.
*   \[LIMPIEZA] Se ha quitado una comprobación de tamaño de archivo que no comprobaba nada.

## 0.9.0 / 2015-01-23

*   \[CAMBIAR] Banderas de línea de comandos reelaboradas, ahora más consistentes y teniendo en cuenta
    necesidades de cuenta del nuevo backend de almacenamiento (consulte a continuación).
*   \[CAMBIAR] Los nombres de las métricas se eliminan después de ciertas transformaciones.
*   \[CAMBIAR] Se ha cambiado la partición de las métricas de resumen exportadas por Prometheus.
*   \[CAMBIAR] Me deshice de Gerrit como herramienta de revisión.
*   \[CAMBIAR] Vista 'tabular' ahora el valor predeterminado (en lugar de 'Gráfico') para evitar
    ejecutando consultas muy costosas accidentalmente.
*   \[CAMBIAR] Se ha cambiado el formato en disco para las muestras almacenadas. Para la actualización, tiene
    para bombardear sus archivos antiguos por completo. Consulte "Reescritura completa del almacenamiento
*   \[CAMBIAR] Se ha eliminado el 2º argumento de `delta`.
*   \[CARACTERÍSTICA] Se ha añadido un `deriv` función.
*   \[CARACTERÍSTICA] Plantillas de consola.
*   \[CARACTERÍSTICA] Añadido `absent` función.
*   \[CARACTERÍSTICA] Permitir omitir el nombre de la métrica en las consultas.
*   \[CORRECCIÓN DE ERRORES] Se han eliminado todas las condiciones de carrera conocidas.
*   \[CORRECCIÓN DE ERRORES] Las mutaciones métricas ahora se manejan correctamente en todos los casos.
*   \[MEJORA] Protección adecuada de doble arranque.
*   \[MEJORA] Reescritura completa de la capa de almacenamiento. Los beneficios incluyen:
    *   Mejor rendimiento de las consultas.
    *   Más muestras en menos RAM.
    *   Mejor gestión de la memoria.
    *   Escala hasta millones de series temporales y miles de muestras ingeridas
        por segundo.
    *   Purga de muestras obsoletas mucho más limpia ahora, hasta completamente
        "olvidando" series temporales obsoletas.
    *   Instrumentación adecuada para diagnosticar la capa de almacenamiento con... pozo...
        Prometeo.
    *   Implementación de Pure Go, ya no hay necesidad de bibliotecas cgo y C compartidas.
    *   Mejor concurrencia.
*   \[MEJORA] Semántica de copia en escritura en la capa AST.
*   \[MEJORA] Cambió de Go 1.3 a Go 1.4.
*   \[MEJORA] Dependencias externas suministradas con godeps.
*   \[MEJORA] Numerosas mejoras en la interfaz de usuario web, trasladadas a Bootstrap3 y
    Rickshaw 1.5.1.
*   \[MEJORA] Integración mejorada de Docker.
*   \[MEJORA] Simplificación del artilugio Makefile.
*   \[LIMPIEZA] Ponga los archivos de metadatos en la forma adecuada (LICENCIA, README.md etc.)
*   \[LIMPIEZA] Se han eliminado todas las advertencias legítimas de 'go vet' y 'golint'.
*   \[LIMPIEZA] Se ha eliminado el código muerto.

## 0.8.0 / 2014-09-04

*   \[MEJORA] Escalona los rasguños para extender la carga.
*   \[CORRECCIÓN DE ERRORES] Cite correctamente el encabezado HTTP Accept.

## 0.7.0 / 2014-08-06

*   \[CARACTERÍSTICA] Se agregaron nuevas funciones: abs(), topk(), bottomk(), drop_common_labels().
*   \[CARACTERÍSTICA] Permita que las plantillas de consola obtengan vínculos de gráficos de expresiones.
*   \[CARACTERÍSTICA] Permita que las plantillas de consola incluyan dinámicamente otras plantillas.
*   \[CARACTERÍSTICA] Las consolas de plantillas ahora tienen acceso a su URL.
*   \[CORRECCIÓN DE ERRORES] Función de tiempo fijo () para devolver el tiempo de evaluación, no el tiempo de bloqueo de pared.
*   \[CORRECCIÓN DE ERRORES] Se ha corregido la pérdida de conexión HTTP cuando los destinos devolvían un estado distinto de 200.
*   \[CORRECCIÓN DE ERRORES] Se ha corregido el vínculo a las plantillas de consola en la interfaz de usuario.
*   \[RENDIMIENTO] Se han eliminado copias de memoria adicionales mientras se raspaban los objetivos.
*   \[MEJORA] Se cambió de Go 1.2.1 a Go 1.3.
*   \[MEJORA] Hizo que las métricas exportadas por el propio Prometheus fueran más consistentes.
*   \[MEJORA] Se han eliminado los retrocesos incrementales para objetivos en mal estado.
*   \[MEJORA] Dockerfile también crea herramientas de soporte de Prometheus ahora.

## 0.6.0 / 2014-06-30

*   \[CARACTERÍSTICA] Se agregó soporte para plantillas de consola y alerta, junto con varias funciones de plantilla.
*   \[RENDIMIENTO] Vaciado en disco mucho más rápido y eficiente en memoria.
*   \[MEJORA] Los resultados de la consulta ahora solo se registran durante la depuración.
*   \[MEJORA] Actualizado a la nueva biblioteca de cliente de Prometheus para exponer métricas.
*   \[CORRECCIÓN DE ERRORES] Las muestras ahora se guardan en la memoria hasta que se vacían completamente en el disco.
*   \[CORRECCIÓN DE ERRORES] Los rasguños de destino que no son 200 ahora se tratan como un error.
*   \[CORRECCIÓN DE ERRORES] Se ha agregado un paso de instalación para la falta de dependencia de Dockerfile.
*   \[CORRECCIÓN DE ERRORES] Se ha eliminado el enlace "Panel de usuario" roto y no utilizado.

## 0.5.0 / 2014-05-28

*   \[CORRECCIÓN DE ERRORES] Se ha corregido la siguiente visualización del tiempo de recuperación en la página de estado.
*   \[CORRECCIÓN DE ERRORES] Se han actualizado algunas referencias de variables en el subdir de herramientas.
*   \[CARACTERÍSTICA] Se agregó soporte para raspar métricas a través del nuevo formato de texto.
*   \[RENDIMIENTO] Rendimiento mejorado del emparejador de etiquetas.
*   \[RENDIMIENTO] Se ha eliminado la sangría JSON en la API de consulta, lo que da lugar a tamaños de respuesta más pequeños.
*   \[MEJORA] Se ha añadido una comprobación interna para verificar el orden temporal de las secuencias.
*   \[MEJORA] Algunas refactorizaciones internas.

## 0.4.0 / 2014-04-17

*   \[CARACTERÍSTICA] Los vectores y escalares ahora pueden invertirse en operaciones binarias (`<scalar> <binop> <vector>`).
*   \[CARACTERÍSTICA] Es posible apagar Prometheus a través de un `/-/quit` punto final web ahora.
*   \[CORRECCIÓN DE ERRORES] Corrección para una condición de carrera de interbloqueo en el almacenamiento de memoria.
*   \[CORRECCIÓN DE ERRORES] Mac OS X compilación fija.
*   \[CORRECCIÓN DE ERRORES] Construido a partir de Go 1.2.1, que tiene correcciones internas para las condiciones de carrera en el manejo de la recolección de basura.
*   \[MEJORA] Refactorización de la interfaz de almacenamiento interno que permite construir, por ejemplo, el `rule_checker` herramienta sin dependencias de biblioteca dinámica de LevelDB.
*   \[MEJORA] Limpiezas en torno al manejo del apagado.
*   \[RENDIMIENTO] Preparativos para una mejor reutilización de la memoria durante el marshaling / unmarshaling.
