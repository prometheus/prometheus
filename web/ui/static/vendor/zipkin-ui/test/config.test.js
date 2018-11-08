import loadConfig from '../js/config';
import {contextRoot} from '../js/publicPath';

const sinon = require('sinon');

describe('Config Data', () => {
  let server;

  before(() => {
    server = sinon.fakeServer.create();
    server.respondImmediately = true;
  });
  after(() => { server.restore(); });

  it('searchEnabled defaults to true', (done) => {
    server.respondWith(`${contextRoot}config.json`, [
      200, {'Content-Type': 'application/json'}, JSON.stringify({})
    ]);

    loadConfig().then(config => {
      config('searchEnabled').should.equal(true);
      done();
    });
  });

  // This tests false can override true!
  it('should parse searchEnabled false value', (done) => {
    server.respondWith(`${contextRoot}config.json`, [
      200, {'Content-Type': 'application/json'}, JSON.stringify(
        {searchEnabled: false}
      )
    ]);

    loadConfig().then(config => {
      config('searchEnabled').should.equal(false);
      done();
    });
  });
});
