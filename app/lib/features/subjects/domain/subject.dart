/// Ids are HashID strings on the wire. They are opaque here on purpose: the
/// server encodes its BIGINT primary keys with a salt and never exposes the
/// integer, so nothing in the app should try to parse one.
class Chapter {
  const Chapter({
    required this.id,
    required this.subjectId,
    required this.title,
    required this.content,
    required this.position,
    this.parentId,
  });

  final String id;
  final String subjectId;
  final String title;

  /// Markdown. The server stores it unchanged, so rendering is the client's job.
  final String content;
  final int position;
  final String? parentId;

  factory Chapter.fromJson(Map<String, dynamic> json) => Chapter(
        id: json['id'] as String,
        subjectId: json['subject_id'] as String? ?? '',
        title: json['title'] as String? ?? '',
        content: json['content'] as String? ?? '',
        position: (json['position'] as num?)?.toInt() ?? 0,
        parentId: json['parent_id'] as String?,
      );
}

class Subject {
  const Subject({
    required this.id,
    required this.title,
    required this.description,
    required this.tags,
    required this.chapters,
  });

  final String id;
  final String title;
  final String description;
  final List<String> tags;
  final List<Chapter> chapters;

  factory Subject.fromJson(Map<String, dynamic> json) => Subject(
        id: json['id'] as String,
        title: json['title'] as String? ?? '',
        description: json['description'] as String? ?? '',
        tags: <String>[...?(json['tags'] as List<dynamic>?)?.map((dynamic tag) => tag as String)],
        chapters: <Chapter>[
          ...?(json['chapters'] as List<dynamic>?)
              ?.map((dynamic chapter) => Chapter.fromJson(chapter as Map<String, dynamic>)),
        ],
      );
}

/// A chapter tree node. Chapters form a tree through `parent_id`, and the API
/// returns them as a flat list, so the client assembles it.
class ChapterNode {
  ChapterNode({required this.chapter, required this.children});

  final Chapter chapter;
  final List<ChapterNode> children;
}

/// Builds the tree in the order the reader expects: siblings sorted by
/// `position`, and a chapter whose parent is missing from the list treated as a
/// root rather than dropped — losing a chapter would hide content.
List<ChapterNode> buildChapterTree(List<Chapter> chapters) {
  final Map<String, List<Chapter>> byParent = <String, List<Chapter>>{};
  final Set<String> known = chapters.map((Chapter chapter) => chapter.id).toSet();
  for (final Chapter chapter in chapters) {
    final String parent =
        (chapter.parentId != null && known.contains(chapter.parentId)) ? chapter.parentId! : '';
    byParent.putIfAbsent(parent, () => <Chapter>[]).add(chapter);
  }

  List<ChapterNode> nodesFor(String parent, Set<String> seen) {
    final List<Chapter> children = <Chapter>[...?byParent[parent]]
      ..sort((Chapter a, Chapter b) => a.position.compareTo(b.position));
    return <ChapterNode>[
      for (final Chapter chapter in children)
        // A cycle in parent_id would otherwise recurse forever.
        if (seen.add(chapter.id))
          ChapterNode(chapter: chapter, children: nodesFor(chapter.id, seen)),
    ];
  }

  final Set<String> emitted = <String>{};
  final List<ChapterNode> roots = nodesFor('', emitted);

  // A cycle in parent_id leaves its members with no path from the root, so they
  // would vanish from the navigator entirely. Surface them as roots: showing a
  // chapter in the wrong place beats losing it.
  final List<Chapter> stranded = <Chapter>[
    for (final Chapter chapter in chapters)
      if (!emitted.contains(chapter.id)) chapter,
  ]..sort((Chapter a, Chapter b) => a.position.compareTo(b.position));
  for (final Chapter chapter in stranded) {
    if (emitted.add(chapter.id)) {
      roots.add(ChapterNode(chapter: chapter, children: nodesFor(chapter.id, emitted)));
    }
  }
  return roots;
}

/// The tree flattened back into reading order, with the nesting depth kept so
/// the list can indent. Used by the chapter navigator.
class FlatChapter {
  const FlatChapter({required this.chapter, required this.depth});

  final Chapter chapter;
  final int depth;
}

List<FlatChapter> flattenChapters(List<ChapterNode> nodes, {int depth = 0}) => <FlatChapter>[
      for (final ChapterNode node in nodes) ...<FlatChapter>[
        FlatChapter(chapter: node.chapter, depth: depth),
        ...flattenChapters(node.children, depth: depth + 1),
      ],
    ];
