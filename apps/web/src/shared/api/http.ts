export async function requestJSON(path: string, options: RequestInit = {}) {
  const response = await fetch(path, options);
  const payload = await response.json();
  return { response, payload };
}
