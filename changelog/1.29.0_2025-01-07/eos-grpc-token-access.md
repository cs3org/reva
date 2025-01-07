Enhancement: access to EOS via tokens over gRPC

As a guest account, accessing a file shared with you relies on a token that is generated on behalf of the resource owner. This method, GenerateToken, has now been implemented in the EOS gRPC client. Additionally, the HTTP client now takes tokens into account.


https://github.com/cs3org/reva/pull/4934