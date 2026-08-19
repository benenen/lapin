class User {
  const User({
    required this.id,
    required this.email,
    required this.name,
    required this.avatarUrl,
    required this.roles,
  });

  final String id;
  final String email;
  final String name;
  final String avatarUrl;
  final List<String> roles;

  bool get isAdmin => roles.contains('admin');

  factory User.fromJson(Map<String, dynamic> json) => User(
        id: json['id'] as String,
        email: json['email'] as String? ?? '',
        name: json['name'] as String? ?? '',
        avatarUrl: json['avatar_url'] as String? ?? '',
        roles: <String>[...?(json['roles'] as List<dynamic>?)?.map((dynamic role) => role as String)],
      );
}
