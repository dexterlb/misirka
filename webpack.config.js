import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

export default {
    mode: 'production',
    entry: {
        'misirka': './ts/index.ts',
    },
    output: {
        path: path.resolve(__dirname, "dist"),
        filename: '[name].min.js',
            libraryTarget: 'umd',
            library: 'Misirka',
            umdNamedDefine: true,
    },
    resolve: {
        extensions: ['.ts', '.js'],
        extensionAlias: {
            '.ts': ['.js', '.ts'],
            '.cts': ['.cjs', '.cts'],
            '.mts': ['.mjs', '.mts']
        },
    },
    module: {
        rules: [
            { test: /\.ts$/, loader: "ts-loader" },
        ],
    },
    optimization: {
        usedExports: true,
    }
}
