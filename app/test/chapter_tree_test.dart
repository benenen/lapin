import 'package:flutter_test/flutter_test.dart';
import 'package:lapin_app/features/subjects/domain/subject.dart';

Chapter chapter(String id, {String? parent, int position = 0, String title = ''}) => Chapter(
      id: id,
      subjectId: 'subject',
      title: title.isEmpty ? id : title,
      content: '',
      position: position,
      parentId: parent,
    );

void main() {
  group('章节树', () {
    test('按 position 排序，而不是接口返回的顺序', () {
      final List<ChapterNode> tree = buildChapterTree(<Chapter>[
        chapter('c', position: 2),
        chapter('a', position: 0),
        chapter('b', position: 1),
      ]);

      expect(tree.map((ChapterNode node) => node.chapter.id), <String>['a', 'b', 'c']);
    });

    test('通过 parent_id 建立层级', () {
      final List<ChapterNode> tree = buildChapterTree(<Chapter>[
        chapter('part', position: 0),
        chapter('ch2', parent: 'part', position: 2),
        chapter('ch1', parent: 'part', position: 1),
        chapter('after', position: 3),
      ]);

      expect(tree.map((ChapterNode node) => node.chapter.id), <String>['part', 'after']);
      expect(tree.first.children.map((ChapterNode node) => node.chapter.id), <String>['ch1', 'ch2']);
    });

    test('父节点缺失时当作根节点，而不是丢掉这一章', () {
      // 分页拉取或权限过滤都可能让父章节不在这一批里；丢弃会让正文凭空消失。
      final List<ChapterNode> tree = buildChapterTree(<Chapter>[
        chapter('orphan', parent: 'missing', position: 1),
        chapter('root', position: 0),
      ]);

      expect(tree.map((ChapterNode node) => node.chapter.id), <String>['root', 'orphan']);
    });

    test('parent_id 成环时不会无限递归', () {
      final List<ChapterNode> tree = buildChapterTree(<Chapter>[
        chapter('a', parent: 'b'),
        chapter('b', parent: 'a'),
      ]);

      // 环里的章节至少要被呈现一次，且调用必须终止。
      expect(tree, isNotEmpty);
    });

    test('展平后保留阅读顺序与缩进深度', () {
      final List<FlatChapter> flat = flattenChapters(buildChapterTree(<Chapter>[
        chapter('part', position: 0),
        chapter('ch1', parent: 'part', position: 1),
        chapter('sub', parent: 'ch1', position: 0),
        chapter('tail', position: 2),
      ]));

      expect(
        flat.map((FlatChapter item) => '${item.chapter.id}@${item.depth}'),
        <String>['part@0', 'ch1@1', 'sub@2', 'tail@0'],
      );
    });
  });

  group('章节解析', () {
    test('顶层章节没有 parent_id', () {
      final Chapter parsed = Chapter.fromJson(<String, dynamic>{
        'id': 'abc',
        'subject_id': 'sub',
        'title': '引言',
        'content': '# 正文',
        'position': 0,
        'parent_id': null,
      });

      expect(parsed.parentId, isNull);
      expect(parsed.content, '# 正文');
    });

    test('缺字段时给出安全默认值，而不是抛异常', () {
      final Chapter parsed = Chapter.fromJson(<String, dynamic>{'id': 'abc'});

      expect(parsed.title, '');
      expect(parsed.position, 0);
    });
  });
}
