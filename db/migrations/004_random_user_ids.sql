SET FOREIGN_KEY_CHECKS = 0;

CREATE TEMPORARY TABLE tmp_user_id_map (old_id BIGINT PRIMARY KEY, new_id BIGINT NOT NULL UNIQUE);

SET @base = FLOOR(10000000 + RAND() * 70000000);
SET @rn = 0;
INSERT INTO tmp_user_id_map (old_id, new_id)
SELECT id, @base + (@rn := @rn + 1)
FROM (SELECT id FROM users ORDER BY RAND()) u, (SELECT @rn := 0) r;

UPDATE messages m JOIN tmp_user_id_map mp ON m.from_user_id = mp.old_id SET m.from_user_id = mp.new_id;
UPDATE messages m JOIN tmp_user_id_map mp ON m.to_user_id   = mp.old_id SET m.to_user_id   = mp.new_id;
UPDATE chat_groups g JOIN tmp_user_id_map mp ON g.owner_id = mp.old_id SET g.owner_id = mp.new_id;
UPDATE group_members gm JOIN tmp_user_id_map mp ON gm.user_id = mp.old_id SET gm.user_id = mp.new_id;
UPDATE friendships f JOIN tmp_user_id_map mp ON f.user_id   = mp.old_id SET f.user_id   = mp.new_id;
UPDATE friendships f JOIN tmp_user_id_map mp ON f.friend_id = mp.old_id SET f.friend_id = mp.new_id;
UPDATE friend_requests fr JOIN tmp_user_id_map mp ON fr.from_user_id = mp.old_id SET fr.from_user_id = mp.new_id;
UPDATE friend_requests fr JOIN tmp_user_id_map mp ON fr.to_user_id   = mp.old_id SET fr.to_user_id   = mp.new_id;
UPDATE group_join_requests gj JOIN tmp_user_id_map mp ON gj.user_id = mp.old_id SET gj.user_id = mp.new_id;

UPDATE users u JOIN tmp_user_id_map m ON u.id = m.old_id SET u.id = m.new_id;

ALTER TABLE users MODIFY id BIGINT NOT NULL;

SET FOREIGN_KEY_CHECKS = 1;
