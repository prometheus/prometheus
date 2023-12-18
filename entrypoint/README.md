# C bindings entrypoint

This folder contains files for building multi-architectural entrypoints for using core functional as library.

## Headers

There is a few header files with public symbols that can be used. All functions optimized for "zero"-size stackframe which is useful for fastcgo-hack. It means that functions take 0–4 arguments as 64-bit pointers and may return values only in fields of arguments. For example, function `add` may has next definition:
```c
// args {
//     a, b int64
// }
// res {
//     result int64
// }
void add(void* args, void* res);
```
which can be used as follows
```c
typedef long long int64_t;

int main() {
    struct {
        int64_t a;
        int64_t b;
    } args;
    struct {
        int64_t result;
    } res;

    args.a = 17;
    args.b = 25;

    add(&args, &res);

    printf("%lld", res.result);
}
```

In most cases we use [aggrement](#aggrement).

## Implementations

// TODO: documenting and write

## Multi-architecture

### Building

// TODO: documenting and add targets to BUILD

### Prefix symbols in archives

// TODO: documenting and make

### Runtime detection

// TODO: documenting and write

### Runtime init

// TODO: documenting and make

## Using

// TODO: documenting

## Contributing

### Aggrement

1. Functions should return `void`.
2. Functions should take 0–4 parameters and all of them should be `void*`.
3. Function declaration should be written in one line.
4. Function comment should explain argument in manner of Go structure notation with Go types and comments.
5. Parameter `args` used to passing data to call. This data shouldn't change.
6. Parameter `res` used to return values. This data should be initialized by caller and will filled by call.
7. We use `uintptr` fields to preserve pointer to internal structures for next calls.
