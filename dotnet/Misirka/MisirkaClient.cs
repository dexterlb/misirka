using System;
using System.Threading;
using System.Threading.Tasks;

namespace Misirka;

// TODO: create overloads using JsonTypeInfo, so Misirka would work on trimmed builds
//  (see RequiresDynamicCodeAttribute, RequiresUnreferencedCodeAttribute) in System.Text.Json

public abstract class MisirkaClient : IAsyncDisposable
{
    public delegate T Parser<out T>(ReadOnlySpan<byte> val);

    public delegate Task AsyncEventHandler<in TEventArgs>(object? sender, TEventArgs e);

    // async event handlers
    public event Func<MisirkaClient, EventArgs, Task>? Alive;
    public event Func<MisirkaClient, EventArgs, Task>? Dead;

    protected async Task OnAlive(EventArgs e) => await Alive?.Raise(this, e)!;
    protected async Task OnDead(EventArgs e) => await Dead?.Raise(this, e)!;

    public abstract Task ConnectAsync(CancellationToken token = default);

    public abstract Task<int> SubscribeAsync<TR>(string topic, AsyncEventHandler<TR> handler);
    public abstract Task UnsubscribeAsync(int id, string topic);
    public abstract Task PingAsync(string? val = null);
    public abstract Task<T> GetAsync<T>(string topic, Parser<T>? parser = null);
    public abstract Task<(int, TR)> CallAsync<TP, TR>(string method, TP param, Parser<TR>? parser = null);
    public abstract Task<int> CommandAsync<TP>(string method, TP param);
    public abstract Task DisconnectAsync(CancellationToken token = default);

    public abstract ValueTask DisposeAsync();
}