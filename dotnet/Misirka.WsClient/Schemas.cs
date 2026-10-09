using System;
using System.Text.Json;

namespace Misirka.WsClient;

internal static class Schemas
{
    public static string Ok(ReadOnlySpan<byte> val) =>
        JsonSerializer.Deserialize<Response<string>>(val)?.Result == "ok"
            ? "ok"
            : throw new Exception("Misirka didn't OK us");

    public static bool Bool(ReadOnlySpan<byte> val) =>
        JsonSerializer.Deserialize<Response<bool>>(val)?.Result ??
        throw new ArgumentException("Cannot parse to boolean");
}