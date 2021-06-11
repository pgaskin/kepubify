// Package zip is an ugly hack to allow kepubify to be build on Go 1.16 with a
// backported archive/zip from Go 1.17 for much better performance. To use it,
// build with `-tags zip117`. This tag does not have any effect on any version
// other than Go 1.16. Note that if other applications embed kepubify and use
// this option, they must also use the same backported module with
// (*kepub.Converter).Convert if they want the performance improvements.
package zip

// TODO: remove this package and the build flags once kepubify switches to Go 1.17
