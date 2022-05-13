import { createProxyMiddleware } from 'http-proxy-middleware';

const setupProxy = (app: any) => {
  app.use(
    '/api',
    createProxyMiddleware({
      target: 'http://localhost:9090',
      changeOrigin: true,
    })
  );
};

export default setupProxy;
