# Use Airplan as a Go library

The `github.com/jimeh/airplan/airplan` package exposes the same document,
collection, revision, rendering, and management operations as the CLI. The CLI
handles local filesystem inference and output formatting; the package owns the
product behavior.

See the [Go reference](https://pkg.go.dev/github.com/jimeh/airplan/airplan) for
every exported type and method. [IMPLEMENTATION.md](../IMPLEMENTATION.md)
describes the repository architecture.

## Upload one document

Load configuration, create a client, and provide an `io.Reader`:

```go
package upload

import (
    "context"
    "fmt"
    "io"

    "github.com/jimeh/airplan/airplan"
)

func document(ctx context.Context, input io.Reader) error {
    cfg, err := airplan.LoadConfig(airplan.ConfigOptions{})
    if err != nil {
        return err
    }

    client, err := airplan.New(ctx, cfg)
    if err != nil {
        return err
    }

    result, err := client.Upload(ctx, airplan.Input{
        Reader: input,
        Name:   "plan.md",
    })
    if err != nil {
        return err
    }

    fmt.Println(result.URL)
    return nil
}
```

`Upload` is the single-page convenience method. Use `UploadDocument` when one
entry owns supporting pages or assets.

## Upload a document bundle

Logical paths are explicit at the library boundary. Filesystem-root and symlink
checks belong to the CLI or another local adapter.

Assets require a seekable reader and declared size so Airplan can stream them
without buffering the complete file:

```go
func bundle(
    ctx context.Context,
    client *airplan.Client,
    entry io.Reader,
    design io.Reader,
    diagram io.ReadSeeker,
    diagramSize int64,
) error {
    result, err := client.UploadDocument(ctx, airplan.DocumentInput{
        Entry: airplan.PageInput{
            Reader: entry,
            Path:   "README.md",
        },
        Pages: []airplan.PageInput{{
            Reader: design,
            Path:   "docs/design.md",
        }},
        Assets: []airplan.AssetInput{{
            Reader:      diagram,
            Path:        "images/flow.svg",
            Size:        diagramSize,
            ContentType: "image/svg+xml",
        }},
    })
    if err != nil {
        return err
    }

    fmt.Println(result.URL)
    fmt.Println(result.Pages[1].URL)
    fmt.Println(result.Assets[0].URL)
    return nil
}
```

`DocumentResult` embeds the entry result and adds ordered `Pages` and `Assets`
slices. `PageInput.Path` and `AssetInput.Path` are validated logical paths.

## Upload a collection

Collection members also use seekable readers and declared sizes:

```go
func collection(ctx context.Context, client *airplan.Client) error {
    image, err := os.Open("screenshot.png")
    if err != nil {
        return err
    }
    defer image.Close()

    info, err := image.Stat()
    if err != nil {
        return err
    }

    result, err := client.UploadFiles(ctx, airplan.FilesInput{
        Files: []airplan.FileInput{{
            Name:   "screenshot.png",
            Reader: image,
            Size:   info.Size(),
        }},
    })
    if err != nil {
        return err
    }

    fmt.Println(result.Files[0].URL)
    fmt.Println(result.URL)
    return nil
}
```

Airplan validates every member before upload and streams file contents from the
declared readers.

## Create a revision

`CreateDocumentRevision` creates a complete replacement revision:

```go
result, err := client.CreateDocumentRevision(
    ctx,
    airplan.CreateDocumentRevisionInput{
        Target: previousURL,
        Document: airplan.DocumentInput{
            Entry: airplan.PageInput{
                Reader: updatedPlan,
                Path:   "plan.md",
            },
        },
    },
)
```

`DocumentRevisionResult` embeds `DocumentResult` and adds `PreviousURL`,
`DiffURL`, and `Unchanged`.

The older `UpdateDocument` method remains as a deprecated, source-compatible
single-page wrapper. There is no separate `UpdateDocumentBundle` method.

## Render without storage

`RenderDocument` renders a complete bundle without storage or manifest writes.
It returns a `RenderedDocumentBundle` containing every generated page and asset
copy operation.

`MaterializeDocument` uses the same renderer, writes through private temporary
files, and publishes a new output directory atomically. Upload and
output-directory preview use this path.

`RenderInput` remains the single-page convenience method.

## Error and cancellation boundaries

Construct clients with `airplan.New`. Nil contexts, nil configuration, and
zero-value clients return errors.

Canceling a context stops Airplan from waiting for a blocked input reader. Go
cannot interrupt an arbitrary `io.Reader`, so a caller that retains the reader
must still unblock or close it.

Invalid document paths, generated-name collisions, and explicit asset
content-type errors support `errors.As` with `InvalidDocumentInputError`. Other
public sentinel errors are documented in the
[Go reference](https://pkg.go.dev/github.com/jimeh/airplan/airplan).

The public library contract is specified in [SPEC.md section
11](../SPEC.md#11-backends-http-server-rest-api-and-mcp).
