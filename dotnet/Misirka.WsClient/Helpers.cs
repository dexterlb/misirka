using System.Text.Json;

namespace System.Runtime.CompilerServices
{
    class IsExternalInit
    {
    }
}

namespace Misirka.WsClient
{
    internal static class Helpers
    {
        public static byte[] ToJsonBytes<T>(this T obj) => JsonSerializer.SerializeToUtf8Bytes(obj);

        // public static void ToJsonBytes<T>(this T obj, Memory<byte> val)
        // {
        //     using var stream = val.AsStream();
        //     JsonSerializer.Serialize(stream, obj);
        // }
    }
}