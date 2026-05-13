export interface Resolver<T> {
  resolve: (val: T) => void;
  reject: (err: any) => void;
}

export interface Error {
  message: string;
  code: number;
}
