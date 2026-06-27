# h77p

A terminal HTTP client driven by `.http` files. Browse and run requests from a keyboard-driven TUI, chain requests with a JavaScript scripting API, and keep offline example responses alongside your definitions.

## Installation

**Prerequisites:** Go 1.22+. Optionally, `jq` in your PATH for response filtering.

```sh
go install github.com/jefferson-julio/h77p@latest
```

Or build from source:

```sh
git clone https://github.com/jefferson-julio/h77p
cd h77p
go build -o h77p .
```

## Usage

```sh
h77p                      # open TUI browser in current directory
h77p ui                   # same, explicit subcommand
h77p run <file> [name]    # run a single request by name
h77p test <file>          # run all requests that have test blocks
```

## Features

- **TUI browser**: navigate directories, pick `.http` files, inspect requests, and run them
- **Request runner**: executes HTTP requests with variable interpolation across URL, headers, and body
- **JavaScript scripting**: pre-request and post-response hooks
- **Test assertions**: write inline tests with `test()` / `assert()` and see pass/fail results
- **`jq` filters**: transform JSON response bodies through one or more chained `@jq` directives
- **`.env` file support**: variables loaded from `.env` files searched upward from the `.http` file's directory
- **Example responses**: save an offline `@example` block per request; shown in the TUI when no live run exists
- **File watching**: the TUI reloads automatically when the `.http` file changes on disk
- **External editor**: open the whole file or a single request block in `$EDITOR`

## .http File Format

A `.http` file contains one or more requests separated by `### Request Name` lines.

```txt
# File-level comment

@baseUrl = https://api.example.com    # file-level variable

### Get Users
GET {{baseUrl}}/users
Accept: application/json
# Comments are allowed in the headers section too

### Create User
@pre-request {%
  request.headers["X-Request-Id"] = "req-" + Date.now();
%}

POST {{baseUrl}}/users
Content-Type: application/json

{
  "name": "Alice",
  "role": "admin"
}

@jq .id                             # extract a field from the JSON response
@jq select(. != null)               # chain filters

@post-response {%
  test("status 201", () => {
    assert(response.status === 201);
  });

  set("userId", String(response.json().id));
  log("created user:", response.jq);
%}

@example {%
  HTTP/1.1 201 Created
  Content-Type: application/json

  {"id": 42, "name": "Alice"}
%}
```

### Variables

Declare variables at file level with `@name = value`. Reference them anywhere in the request with `{{name}}`. Variables set by `set()` in a post-response script are available to all subsequent requests in the session.

```http
@baseUrl = https://api.example.com
@token   = secret

GET {{baseUrl}}/profile
Authorization: Bearer {{token}}
```

Variable resolution order (last wins): `.env` files → `@var` declarations → `set()` calls.

### `@jq` Filters

Append one or more `@jq` lines after the headers/body to pipe the JSON response through `jq`. Filters are chained.

```txt
GET https://api.example.com/posts

@jq .[]
@jq select(.userId == 1)
@jq .title
```

Requires `jq` in PATH and a JSON `Content-Type` response. The filtered result is shown in the **Run** tab and is available as `response.jq` in post-response scripts.

### `@example` Blocks

Save an offline example response to show in the TUI when no live run has been made. The block is written in raw HTTP response format:

```txt
@example {%
  HTTP/1.1 200 OK
  Content-Type: application/json

  {"id": 1}
%}
```

Press `x` in the TUI to run the request and save its response (or jq output) as the example automatically.

## JavaScript Scripting API

Scripts run in an embedded ES5.1+ engine ([goja](https://github.com/dop251/goja)). Both hook types share the same global scope.

### `@pre-request {%...%}`

Runs before the request is sent. Use it to mutate headers, the URL, or the body.

```javascript
request.headers["Authorization"] = "Bearer " + env.token;
request.headers["X-Timestamp"]   = String(Date.now());
```

### `@post-response {%...%}`

Runs after the response is received. Use it to assert behaviour, capture values, and log output.

```javascript
test("returns 200", () => {
  assert(response.status === 200, "expected 200, got " + response.status);
});

set("authToken", response.json().token);
log("captured token:", env.authToken);
```

### Global objects

| Object | Available in | Description |
|--------|-------------|-------------|
| `request` | both | The outgoing request |
| `response` | post-response | The received response |
| `env` | both | All variables in the current session |

### `request`

| Property | Type | Description |
|----------|------|-------------|
| `.method` | `string` | HTTP method (`"GET"`, `"POST"`, …) |
| `.url` | `string` | Full URL after variable interpolation |
| `.headers` | `object` | Request headers map |
| `.body` | `string` | Raw request body |

### `response`

| Property | Type | Description |
|----------|------|-------------|
| `.status` | `number` | HTTP status code |
| `.statusText` | `string` | Status text (`"OK"`, `"Not Found"`, …) |
| `.headers` | `object` | Response headers map |
| `.body` | `string` | Raw response body as a string |
| `.duration` | `number` | Round-trip time in milliseconds |
| `.json()` | `function` | Parses `.body` as JSON and returns the value |
| `.jq` | `any` | Result of `@jq` filters, parsed JSON or string fallback; `null` if no filters defined |

### Functions

| Function | Description |
|----------|-------------|
| `test(name, fn)` | Register a named test. `fn` is called immediately; failures are collected and shown in the **Tests** tab. |
| `assert(cond, msg?)` | Throw (failing the current test) if `cond` is falsy. `msg` is optional. |
| `set(key, value)` | Write a variable into the shared session env. Available as `{{key}}` in later requests and as `env.key` in scripts. |
| `log(...args)` | Append formatted output to the **Logs** tab. |

### `env`

A plain object containing all variables currently in scope. Read any variable with `env.varName`. Variables come from `.env` files, `@var` declarations, and previous `set()` calls.

```javascript
log(env.baseUrl);          // read a declared variable
set("step", "1");          // write — visible to all subsequent requests
log(env.step);             // "1"
```
