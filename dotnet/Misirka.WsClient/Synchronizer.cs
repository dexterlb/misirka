using System;
using System.Threading;
using System.Threading.Tasks;

namespace Misirka.WsClient;

internal class Synchronizer<T>(T initial, Func<T, T> updater) where T: struct
{
    private readonly SemaphoreSlim _lock = new(1, 1);

    private T _lastValue = initial;

    public T GetNext()
    {
        _lock.Wait();
        _lastValue = updater(_lastValue);
        var temp = _lastValue;
        _lock.Release();
        return temp;
    }

    public async Task<T> GetNextAsync()
    {
        await _lock.WaitAsync();
        _lastValue = updater(_lastValue);
        var temp = _lastValue;
        _lock.Release();
        return temp;
    }
}