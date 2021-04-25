# SFN concurrency control

_Step functions_ allow you to create a robust concurrency control mechanism on top of _Dynamodb_.
You would most likely use it when the concurrency control via `Map` state is not possible.

## Considerations

### Concurrent updates

To ensure that we are maintaining the concurrency counter, our `updateItem` operations contain _ConditionExpression_.
It may happen that the update will be rejected because some other operation is changing given item.
These operations should usually be retried, and we are doing that with the combination of `Catch` and `Retry` constructs.

### Why single item

I normally avoid having multiple operations (potentially concurrent) working on the same DB item.
All would be nice if only the _Step Functions_ natively allowed for the DDB `Query` operation. Sadly this is not supported.

We could have each execution perform a `Query` operation and check the amount of items (in our case up to 5) exists within the table.
The aforementioned limitation means that we would have to use Lambda for that.

I would personally be fine with this approach but it seems kina weird that we want to control concurrency of one lambda while not
controlling the concurrency of the lambda that performs the `Query` operation.
