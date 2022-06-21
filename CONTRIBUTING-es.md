# Contribuyendo

Prometheus utiliza GitHub para gestionar las revisiones de las solicitudes de extracción.

*   Si eres un nuevo colaborador, consulta: [Pasos para contribuir](#steps-to-contribute)

*   Si tiene una solución o mejora trivial, continúe y cree una solicitud de extracción,
    direccionamiento (con `@...`) un mantenedor adecuado de este repositorio (véase
    [MAINTAINERS.md](MAINTAINERS.md)) en la descripción de la solicitud de extracción.

*   Si planeas hacer algo más complicado, primero discute tus ideas
    en nuestro [lista de correo](https://groups.google.com/forum/?fromgroups#!forum/prometheus-developers).
    Esto evitará trabajos innecesarios y seguramente le dará a usted y a nosotros un buen trato.
    de inspiración. También por favor vea nuestro [problema de no goles](https://github.com/prometheus/docs/issues/149) en áreas en las que la comunidad de Prometheus no planea trabajar.

*   Las pautas de estilo de codificación relevantes son las [Revisión de código de Go
    Comentarios](https://code.google.com/p/go-wiki/wiki/CodeReviewComments)
    y el *Formato y estilo* sección de Peter Bourgon [Ir: Mejor
    Prácticas para la producción
    Entornos](https://peter.bourgon.org/go-in-production/#formatting-and-style).

*   Asegúrese de firmar el [DCO](https://github.com/probot/dco#how-it-works).

## Pasos para contribuir

Si desea trabajar en un problema, reclame primero comentando el problema de GitHub que desea trabajar en él. Esto es para evitar la duplicación de esfuerzos de los contribuyentes sobre el mismo tema.

Por favor, compruebe el [`low-hanging-fruit`](https://github.com/prometheus/prometheus/issues?q=is%3Aissue+is%3Aopen+label%3A%22low+hanging+fruit%22) para encontrar problemas que sean buenos para comenzar. Si tiene preguntas sobre uno de los problemas, con o sin la etiqueta, comente sobre ellos y uno de los mantenedores lo aclarará. Para una respuesta más rápida, contáctenos a través de [IRC](https://prometheus.io/community).

Puedes [Poner en marcha un entorno de desarrollo prediseñado](https://gitpod.io/#https://github.com/prometheus/prometheus) utilizando Gitpod.io.

Para obtener instrucciones completas sobre cómo compilar, consulte: [Construyendo desde la fuente](https://github.com/prometheus/prometheus#building-from-source)

Para compilar y probar rápidamente los cambios:

```bash
# For building.
go build ./cmd/prometheus/
./prometheus

# For testing.
make test         # Make sure all the tests pass before you commit and push :)
```

Utilizamos [`golangci-lint`](https://github.com/golangci/golangci-lint) para linting el código. Si informa de un problema y cree que la advertencia debe ignorarse o es un falso positivo, puede agregar un comentario especial `//nolint:linter1[,linter2,...]` antes de la línea ofensiva. Sin embargo, use esto con moderación, arreglar el código para cumplir con la recomendación del linter es, en general, el curso de acción preferido.

Todos nuestros problemas se etiquetan regularmente para que también pueda filtrar los problemas relacionados con los componentes en los que desea trabajar. Para nuestra política de etiquetado, consulte [la página wiki](https://github.com/prometheus/prometheus/wiki/Label-Names-and-Descriptions).

## Lista de comprobación de solicitudes de extracción

*   Ramifique desde la rama principal y, si es necesario, vuelva a basarse en la rama principal actual antes de enviar su solicitud de extracción. Si no se fusiona limpiamente con main, es posible que se le pida que vuelva a basar sus cambios.

*   Las confirmaciones deben ser lo más pequeñas posible, al tiempo que se garantiza que cada confirmación sea correcta de forma independiente (es decir, cada confirmación debe compilar y pasar pruebas).

*   Si su parche no se está revisando o necesita que una persona específica lo revise, puede @-responder a un revisor solicitando una revisión en la solicitud de extracción o un comentario, o puede solicitar una revisión en el canal IRC [#prometheus-dev](https://web.libera.chat/?channels=#prometheus-dev) en irc.libera.chat (para el inicio más fácil, [unirse a través de Element](https://app.element.io/#/room/#prometheus-dev:matrix.org)).

*   Agregue pruebas relevantes para el error corregido o la nueva característica.

## Gestión de dependencias

El proyecto Prometheus utiliza [Módulos Go](https://golang.org/cmd/go/#hdr-Modules\_\_module_versions\_\_and_more) para administrar dependencias en paquetes externos.

Para agregar o actualizar una nueva dependencia, utilice el botón `go get` mandar:

```bash
# Pick the latest tagged release.
go install example.com/some/module/pkg@latest

# Pick a specific version.
go install example.com/some/module/pkg@vX.Y.Z
```

Ordenar el `go.mod` y `go.sum` Archivos:

```bash
# The GO111MODULE variable can be omitted when the code isn't located in GOPATH.
GO111MODULE=on go mod tidy
```

Tienes que confirmar los cambios en `go.mod` y `go.sum` antes de enviar la solicitud de extracción.
