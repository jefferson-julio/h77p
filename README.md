https://github.com/user-attachments/assets/424e3eeb-1492-4412-8ca2-1dd7957285ca



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
h77p run <file> [name]    # run a single request by name
h77p test <file>          # run all requests that have test blocks
```

## Features

- **TUI browser**: navigate directories, pick `.http` files, inspect requests, and run them
- **Request runner**: executes HTTP requests with variable interpolation across URL, headers, and body
- **Inline JS expressions**: use `${{expr}}` anywhere in the URL, headers, or body
- **JavaScript scripting**: pre-request and post-response hooks with a rich stdlib
- **Test assertions**: write inline tests with `test()` / `assert()` and see pass/fail results
- **`jq` filters**: transform JSON response bodies through one or more chained `@jq` directives
- **Example responses**: save an offline `@example` block per request
- **File watching**: the TUI reloads automatically when the `.http` file changes on disk
- **External editor**: open the whole file or a single request block in `$EDITOR`
- **External body viewer**: open the response body in [otree](https://github.com/fioncat/otree), fallback to [jless](https://github.com/PaulJuliusMartinez/jless), then `less`

## File Format

See [`testdata/sample.http`](./testdata/sample.http) for a full annotated example covering variables, groups, scripts, `@jq` filters, multipart uploads, inline expressions, and saved examples. Supporting files in [`testdata/`](./testdata/) show date/fake data, XML, and nested group imports.

## JavaScript Scripting API

Scripts run in an embedded ES5.1+ engine ([goja](https://github.com/dop251/goja)). Globals marked **pre** are available in `@pre-request` blocks; **post** in `@post-response` blocks; **both** in either.

### Hook blocks

```javascript
// @pre-request {% runs before the request is sent
request.headers["Authorization"] = "Bearer " + env.token;
request.url = request.url + "?ts=" + date.now().unix();
// %}

// @post-response {% runs after the response is received
test("status 200", () => {
  assert(response.status === 200);
});
set("token", response.json().access_token);
log("captured:", env.token);
// %}
```

### Core globals

#### `request` (both)

| Property | Type | Description |
|----------|------|-------------|
| `.method` | string | HTTP method (`"GET"`, `"POST"`, …) |
| `.url` | string | Full URL after variable interpolation |
| `.headers` | object | Mutable request headers map |
| `.body` | string | Raw request body |

#### `response` (post)

| Property | Type | Description |
|----------|------|-------------|
| `.status` | number | HTTP status code |
| `.statusText` | string | Status text (`"OK"`, `"Not Found"`, …) |
| `.headers` | object | Response headers map |
| `.body` | string | Raw response body |
| `.duration` | number | Round-trip time in milliseconds |
| `.json()` | function | Parse `.body` as JSON |
| `.jq` | any | Result of `@jq` filters; undefined when no filters defined |

#### `env` (both)

A plain object with all session variables. Read with `env.varName`. Variables come from `.env` files, `@var` declarations, and `set()` calls.

### Functions

| Function | Available | Description |
|----------|-----------|-------------|
| `test(name, fn)` | post | Register a named test; `fn` runs immediately, failures go to the Tests tab |
| `assert(cond, msg?)` | post | Throw (failing the current test) if `cond` is falsy |
| `set(key, value)` | both | Write a variable into the shared session env |
| `log(...args)` | both | Append output to the Logs tab |
| `onSuccess(fn)` | post | Call `fn` after the script only if every `test()` passed |

`set()` and `${{expr}}` coerce values automatically: date objects → ISO 8601, numbers → decimal string, booleans → `"true"` / `"false"`, null/undefined → `""`.

### `fake` (both)

Backed by [gofakeit](https://github.com/brianvoe/gofakeit).

```javascript
fake.name()           // "John Doe"
fake.firstName()      // "John"
fake.lastName()       // "Doe"
fake.email()          // "john@example.com"
fake.uuid()           // "550e8400-..."
fake.username()       // "user123"
fake.password(12)     // random password, length 12 (default 12)
fake.phone()          // "+15551234567"
fake.url()            // "https://example.com/path"
fake.ipv4()           // "192.168.1.1"
fake.color()          // "#a3f8c2"
fake.hex(8)           // "a3f8c2d1" n hex chars (default 8)
fake.int(0, 100)      // random int in range (defaults: 0–100)
fake.float(0.0, 1.0)  // random float in range (defaults: 0–100)
fake.bool()           // true or false
fake.word()           // "laptop"
fake.sentence(6)      // sentence with n words (default 6)
fake.paragraph()      // 1 paragraph, ~3 sentences
fake.city()           // "Springfield"
fake.country()        // "United States"
fake.address()        // { street, city, state, zip, country }
fake.date()           // ISO 8601 string of a random date
fake.pastDate()       // random date in the past 5 years
fake.futureDate()     // random date in the next 5 years
```

### `date` (both)

Moment.js-style date manipulation.

```javascript
// Constructors
date.now()                   // current time
date.parse("2024-01-15")     // from string (ISO 8601, MM/DD/YYYY, DD-MM-YYYY, …)
date.unix(1705276800)        // from unix seconds
date.unixMs(1705276800000)   // from unix milliseconds

// Arithmetic, returns a new date object
d.add(1, "day")
d.subtract(2, "hours")
// Units: years y  months M  weeks w  days d  hours h  minutes m  seconds s  milliseconds ms

// Formatting
d.format("YYYY-MM-DD")             // "2024-01-15"
d.format("YYYY-MM-DDTHH:mm:ss")    // "2024-01-15T10:30:00"
// Tokens: YYYY YY MM M DD D HH hh h mm m ss s A a Z

// Boundaries
d.startOf("month")   d.endOf("week")
// Units: year y  month M  week w  day d  hour h  minute m

// Comparison
d.isBefore(other)    d.isAfter(other)
d.diff(other, "days")   // signed integer (self − other)

// Output
d.unix()          // unix seconds
d.unixMs()        // unix milliseconds
d.toISOString()   // "2024-01-15T10:30:00Z"

// Components
d.year()  d.month()  d.day()  d.weekday()
d.hour()  d.minute() d.second()
```

### `jwt` (both)

Decode and inspect JWTs without verifying the signature.

```javascript
var tok = jwt.decode(response.json().access_token);
tok.header.alg      // "HS256"
tok.payload.sub     // "user-123"
tok.payload.exp     // 1234567890

jwt.isExpired(token)    // true when exp is in the past
jwt.expiresAt(token)    // date object for exp claim, or null

// Common pattern
set("authToken", response.json().access_token);
log("expires in " + jwt.expiresAt(env.authToken).diff(date.now(), "hours") + "h");
```

### `xml` (both)

```javascript
var doc = xml.parse(response.body);
doc.name              // element tag name
doc.attrs.id          // attribute by name
doc.text              // trimmed text content
doc.children          // array of child nodes
doc.find("item")      // first child named "item", or null
doc.findAll("item")   // all children named "item"
```

### Inline expressions — `${{expr}}`

Any `${{expr}}` token in the URL, headers, or body is evaluated as a JavaScript expression at run time, after `{{variable}}` interpolation. All `fake`, `date`, `jwt`, `xml`, and `env` globals are available.

```http
### Create User
POST /users/${{fake.uuid()}}
Content-Type: application/json
X-Timestamp: ${{date.now().unix()}}

{
  "name": "${{fake.name()}}",
  "expiry": "${{date.now().add(30, 'days').toISOString()}}"
}
```
