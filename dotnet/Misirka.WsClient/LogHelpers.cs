using System;
using JetBrains.Annotations;
using Microsoft.Extensions.Logging;

namespace Misirka.WsClient;

// otherwise we need to write a custom code analyzer
#pragma warning disable CA2254

internal static class LogHelpers
{
    public static void LogIf<T>(this ILogger<T>? logger, LogLevel level, [StructuredMessageTemplate] string? msg,
        params object[] args)
    {
        if (logger?.IsEnabled(level) is true) logger.Log(level, msg, args);
    }

    public static void LogIf<T>(this ILogger<T>? logger, LogLevel level, Exception? exception,
        [StructuredMessageTemplate] string? msg, params object[] args)
    {
        if (logger?.IsEnabled(level) is true) logger.Log(level, exception, msg, args);
    }

    public static void LogIf<T>(this ILogger<T>? logger, LogLevel level, EventId eventId, Exception? exception,
        [StructuredMessageTemplate] string? msg, params object[] args)
    {
        if (logger?.IsEnabled(level) is true) logger.Log(level, eventId, exception, msg, args);
    }

    public static void LogIf<T>(this ILogger<T>? logger, LogLevel level, EventId eventId,
        [StructuredMessageTemplate] string? msg, params object[] args)
    {
        if (logger?.IsEnabled(level) is true) logger.Log(level, eventId, msg, args);
    }
}
#pragma warning restore CA2254