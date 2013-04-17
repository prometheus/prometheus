# Important Notes
This package is unidiomatic for Go in a few ways:

1. Its build step is to ``make install`` versus just be compiled and referenced
   by the other packages.

2. In an ideal world, this will be moved into the same package as the LevelDB
   implementation of metric storage.

These are considerations worth remedying, but it is also important to weigh this
against spreading pain between the C/C++ world and the rest of the pure Go
implementation.