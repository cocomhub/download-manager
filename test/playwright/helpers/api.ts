import { request, type RequestOptions } from 'http';

const TEST_PORT = parseInt(process.env.TEST_PORT || '19199', 10);

export async function apiGet<T = any>(path: string): Promise<T> {
  return apiRequest<T>('GET', path);
}

export async function apiPost<T = any>(path: string, body?: any): Promise<T> {
  return apiRequest<T>('POST', path, body);
}

export async function apiPatch<T = any>(path: string, body?: any): Promise<T> {
  return apiRequest<T>('PATCH', path, body);
}

function apiRequest<T>(method: string, path: string, body?: any): Promise<T> {
  return new Promise((resolve, reject) => {
    const options: RequestOptions = {
      hostname: 'localhost',
      port: TEST_PORT,
      path,
      method,
      headers: { 'Content-Type': 'application/json' },
    };

    const req = request(options, (res) => {
      let data = '';
      res.on('data', (chunk) => { data += chunk; });
      res.on('end', () => {
        try {
          resolve(JSON.parse(data));
        } catch {
          resolve(data as unknown as T);
        }
      });
    });

    req.on('error', reject);

    if (body !== undefined) {
      req.write(JSON.stringify(body));
    }
    req.end();
  });
}
