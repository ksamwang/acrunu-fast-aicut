import { message } from "antd";
import { useEffect, useState, type DependencyList } from "react";
import { apiRequest } from "../api/server-api";

type ResourceLoader<T> = (path: string, token: string) => Promise<T>;

export function useResource<T>(
  path: string | null,
  token: string,
  deps: DependencyList = [],
  load: ResourceLoader<T> = (resourcePath, authToken) => apiRequest<T>(resourcePath, {}, authToken)
) {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(false);

  const reload = async () => {
    if (!path) {
      setData(null);
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      setData(await load(path, token));
    } catch (error) {
      message.error(error instanceof Error ? error.message : "加载失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void reload();
  }, [path, ...deps]);

  return { data, loading, reload };
}
