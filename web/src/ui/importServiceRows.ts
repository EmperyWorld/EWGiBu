export type ImportServiceRow = {
  id: string;
  user: string;
  environmentFrom: string;
};

type ServiceFields = {
  user?: string;
  environmentFrom?: string;
};

export function importServiceRows(
  services: Record<string, ServiceFields> | undefined,
): ImportServiceRow[] {
  if (!services) return [];
  return Object.keys(services)
    .sort()
    .filter((id) => {
      const svc = services[id];
      return Boolean(svc?.user || svc?.environmentFrom);
    })
    .map((id) => ({
      id,
      user: services[id]?.user ?? "",
      environmentFrom: services[id]?.environmentFrom ?? "",
    }));
}
