using System;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;

namespace Misirka;

public static class Helpers
{
    public static Task Raise<TSource, TEventArgs>(this Func<TSource, TEventArgs, Task>? handlers, TSource source,
        TEventArgs args)
        where TEventArgs : EventArgs
    {
        if (handlers is not null)
        {
            return Task.WhenAll(handlers.GetInvocationList()
                .OfType<Func<TSource, TEventArgs, Task>>()
                .Select(h => h(source, args)));
        }

        return Task.CompletedTask;
    }

    public static CancellationToken SecondsToTimeout(double? seconds) =>
        seconds.HasValue ? new CancellationTokenSource(TimeSpan.FromSeconds(seconds.Value)).Token : CancellationToken.None;
}