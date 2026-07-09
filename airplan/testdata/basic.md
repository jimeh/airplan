# Refactor auth

A short plan document exercising the rendering pipeline.

## Goals

- Replace session cookies with tokens
- Keep the ~~old~~ legacy login working
- Ship behind a flag[^flag]

## Tasks

- [x] Write the spec
- [ ] Implement the middleware

## Code

```go
func main() {
	fmt.Println("hello")
}
```

```
plain block, no language
```

## Matrix

| Case     | Result |
| -------- | ------ |
| happy    | pass   |
| sad      | fail   |

> [!NOTE]
> The rollback plan lives at https://example.com/rollback.

[^flag]: `auth_v2` in the flags service.
