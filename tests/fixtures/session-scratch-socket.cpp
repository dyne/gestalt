// Copyright (C) 2026 Dyne.org foundation
// SPDX-License-Identifier: AGPL-3.0-or-later
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>
#include <cassert>
#include <cstring>
#include <filesystem>
#include <iostream>
#include <string>

int main() {
    const auto root = std::filesystem::temp_directory_path();
    assert(std::filesystem::canonical(root) == root);
    assert(root.string().size() <= 48);
    const std::string path = (root / "bitcoin_sock0_qR2DkZ").string();
    sockaddr_un address{};
    address.sun_family = AF_UNIX;
    assert(path.size() + 1 <= sizeof(address.sun_path));
    std::memcpy(address.sun_path, path.c_str(), path.size() + 1);
#if defined(__APPLE__)
    address.sun_len = sizeof(address);
#endif
    const int server = socket(AF_UNIX, SOCK_STREAM, 0);
    assert(server >= 0);
    assert(bind(server, reinterpret_cast<sockaddr*>(&address), sizeof(address)) == 0);
    assert(listen(server, 1) == 0);
    const int client = socket(AF_UNIX, SOCK_STREAM, 0);
    assert(client >= 0);
    assert(connect(client, reinterpret_cast<sockaddr*>(&address), sizeof(address)) == 0);
    assert(send(client, "ping", 4, 0) == 4);
    const int peer = accept(server, nullptr, nullptr);
    assert(peer >= 0);
    char bytes[4];
    assert(recv(peer, bytes, 4, MSG_WAITALL) == 4);
    assert(std::memcmp(bytes, "ping", 4) == 0);
    assert(send(peer, "pong", 4, 0) == 4);
    assert(recv(client, bytes, 4, MSG_WAITALL) == 4);
    assert(std::memcmp(bytes, "pong", 4) == 0);
    close(peer); close(client); close(server);
    assert(unlink(path.c_str()) == 0);
    std::cout << "cpp-socket-ok " << sizeof(address.sun_path) << '\n';
}
