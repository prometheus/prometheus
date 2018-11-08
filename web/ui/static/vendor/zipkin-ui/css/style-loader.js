// We use this file to let webpack generate
// a css bundle from our stylesheets.

// The import of 'publicPath' module has to be the first statement in this entry point file
// so that '__webpack_public_path__' (see https://webpack.github.io/docs/configuration.html#output-publicpath)
// is set soon enough.
// In the same time, 'contextRoot' is made available as the context root path reference.
import {contextRoot} from '../js/publicPath';

require('./main.scss');
