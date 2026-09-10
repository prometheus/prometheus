# Making changes to textparse lexers

In the rare case that you need to update the textparse lexers, edit `promlex.l`
or `openmetricslex.l`. The root `Makefile` pins `modernc.org/golex` v1.1.0 and
provides the following regeneration targets, which you can run from the
repository root:

```sh
make install-golex generate-textparse-lexers
```

This regenerates both `promlex.l.go` and `openmetricslex.l.go`.
