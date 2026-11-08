import type { FileUpload } from '../service/types';

/** Reads a file the user chose in an <input type="file">. */
export async function readUpload(file: File): Promise<FileUpload> {
  return { name: file.name, data: new Uint8Array(await file.arrayBuffer()) };
}
