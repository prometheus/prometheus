// Custom Jest resolver to handle @prometheus-io/lezer-promql/client subpath export
const path = require('path');

// The lezer-promql package is in the workspace node_modules (web/ui/node_modules)
const lezerPromqlBase = path.resolve(__dirname, '../../node_modules/@prometheus-io/lezer-promql');

module.exports = function (request, options) {
  // Handle the lezer-promql/client subpath export
  if (request === '@prometheus-io/lezer-promql/client') {
    return path.join(lezerPromqlBase, 'dist/cjs/client/index.cjs');
  }

  // Handle the main lezer-promql import
  if (request === '@prometheus-io/lezer-promql') {
    return path.join(lezerPromqlBase, 'dist/index.cjs');
  }

  // Use the default resolver for everything else
  return options.defaultResolver(request, options);
};
